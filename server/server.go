package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "github.com/lbgsct/cryptokurs/proto/chatpb"
)

// Структура клиента
type Client struct {
	ClientID  string
	PublicKey string // Публичный ключ в формате hex
	SharedKey []byte
}

// Структура комнаты
type ChatRoom struct {
	RoomID    string
	Algorithm string
	Mode      string
	Padding   string
	Prime     string
	Clients   map[string]*Client
	mutex     sync.RWMutex
}

// Структура сервера
type ChatServer struct {
	chatpb.UnimplementedChatServiceServer
	redisStore      *RedisStore
	rooms           map[string]*ChatRoom
	roomsMutex      sync.RWMutex
	rabbitMQConn    *amqp.Connection
	rabbitMQChannel *amqp.Channel
}

// Создание нового сервера
func NewChatServer() *ChatServer {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Не удалось подключиться к RabbitMQ: %v", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("Не удалось открыть канал RabbitMQ: %v", err)
	}

	// Инициализация RedisStore
	redisStore := NewRedisStore("localhost:6379", "", 0)

	return &ChatServer{
		redisStore:      redisStore,
		rooms:           make(map[string]*ChatRoom),
		rabbitMQConn:    conn,
		rabbitMQChannel: channel,
	}
}

// Helper function to generate a random room ID
func generateRoomID() string {
	bytes := make([]byte, 4) // 4 bytes дадут 8-символьную hex-строку
	_, err := rand.Read(bytes)
	if err != nil {
		log.Fatalf("Не удалось сгенерировать случайные байты: %v", err)
	}
	return hex.EncodeToString(bytes)
}

// Метод создания комнаты
func (s *ChatServer) CreateRoom(ctx context.Context, req *chatpb.CreateRoomRequest) (*chatpb.CreateRoomResponse, error) {
	roomID := generateRoomID()
	s.roomsMutex.Lock()
	defer s.roomsMutex.Unlock()

	s.rooms[roomID] = &ChatRoom{
		RoomID:    roomID,
		Algorithm: req.Algorithm,
		Mode:      req.Mode,
		Padding:   req.Padding,
		Prime:     req.Prime,
		Clients:   make(map[string]*Client),
	}

	// Создание комнаты в Redis (обновите эту часть при необходимости)
	err := s.redisStore.CreateRoom(ctx, roomID, req.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("Не удалось создать комнату в Redis: %v", err)
	}

	// Объявление exchange типа fanout для комнаты
	err = s.rabbitMQChannel.ExchangeDeclare(
		roomID,   // имя exchange (например, ID комнаты)
		"fanout", // тип exchange
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("Не удалось объявить exchange: %v", err)
	}

	return &chatpb.CreateRoomResponse{
		RoomId: roomID,
	}, nil
}

// Метод получения параметров комнаты
func (s *ChatServer) GetRoom(ctx context.Context, req *chatpb.GetRoomRequest) (*chatpb.GetRoomResponse, error) {
	roomID := req.GetRoomId()
	s.roomsMutex.RLock()
	defer s.roomsMutex.RUnlock()

	room, ok := s.rooms[roomID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "Комната с ID %s не найдена", roomID)
	}

	return &chatpb.GetRoomResponse{
		Algorithm: room.Algorithm,
		Mode:      room.Mode,
		Padding:   room.Padding,
		Prime:     room.Prime,
	}, nil
}

// Метод закрытия комнаты
func (s *ChatServer) CloseRoom(ctx context.Context, req *chatpb.CloseRoomRequest) (*chatpb.CloseRoomResponse, error) {
	s.roomsMutex.Lock()
	defer s.roomsMutex.Unlock()

	room, exists := s.rooms[req.RoomId]
	if !exists {
		return &chatpb.CloseRoomResponse{
			Success: false,
		}, fmt.Errorf("Комната не найдена")
	}

	// Удаление exchange (fanout)
	err := s.rabbitMQChannel.ExchangeDelete(
		room.RoomID,
		false, // ifUnused
		false, // nowait
	)
	if err != nil {
		return nil, fmt.Errorf("Не удалось удалить exchange: %v", err)
	}

	// Удаление комнаты из RedisStore
	err = s.redisStore.DeleteRoom(ctx, req.RoomId)
	if err != nil {
		return nil, fmt.Errorf("Не удалось удалить комнату из Redis: %v", err)
	}

	delete(s.rooms, req.RoomId)

	return &chatpb.CloseRoomResponse{
		Success: true,
	}, nil
}

// Метод присоединения к комнате
// Метод присоединения к комнате с ограничением на максимальное количество клиентов (2)
func (s *ChatServer) JoinRoom(ctx context.Context, req *chatpb.JoinRoomRequest) (*chatpb.JoinRoomResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return &chatpb.JoinRoomResponse{
			Success: false,
			Error:   "Комната не найдена",
		}, nil
	}

	room.mutex.Lock()
	defer room.mutex.Unlock()

	// Проверка, достигнуто ли максимальное количество клиентов
	if len(room.Clients) >= 2 {
		return &chatpb.JoinRoomResponse{
			Success: false,
			Error:   "Комната уже заполнена",
		}, nil
	}

	if _, exists := room.Clients[req.ClientId]; exists {
		return &chatpb.JoinRoomResponse{
			Success: false,
			Error:   "Клиент уже присоединился к комнате",
		}, nil
	}

	room.Clients[req.ClientId] = &Client{
		ClientID:  req.ClientId,
		PublicKey: "",
	}

	// Добавление клиента в RedisStore (если требуется)
	err := s.redisStore.AddClientToRoom(ctx, req.RoomId, req.ClientId)
	if err != nil {
		return &chatpb.JoinRoomResponse{
			Success: false,
			Error:   "Не удалось добавить клиента в комнату",
		}, nil
	}

	log.Printf("Клиент %s присоединился к комнате %s", req.ClientId, req.RoomId)

	return &chatpb.JoinRoomResponse{
		Success: true,
	}, nil
}

// Метод выхода из комнаты
func (s *ChatServer) LeaveRoom(ctx context.Context, req *chatpb.LeaveRoomRequest) (*chatpb.LeaveRoomResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return &chatpb.LeaveRoomResponse{
			Success: false,
			Error:   "Комната не найдена",
		}, nil
	}

	room.mutex.Lock()
	defer room.mutex.Unlock()

	if _, exists := room.Clients[req.ClientId]; !exists {
		return &chatpb.LeaveRoomResponse{
			Success: false,
			Error:   "Клиент не найден в комнате",
		}, nil
	}

	delete(room.Clients, req.ClientId)

	// Удаление клиента из RedisStore (если требуется)
	err := s.redisStore.RemoveClientFromRoom(ctx, req.RoomId, req.ClientId)
	if err != nil {
		return &chatpb.LeaveRoomResponse{
			Success: false,
			Error:   "Не удалось удалить клиента из комнаты",
		}, nil
	}

	return &chatpb.LeaveRoomResponse{
		Success: true,
	}, nil
}

// Метод отправки публичного ключа
func (s *ChatServer) SendPublicKey(ctx context.Context, req *chatpb.SendPublicKeyRequest) (*chatpb.SendPublicKeyResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "Комната не найдена",
		}, nil
	}

	room.mutex.Lock()
	client, exists := room.Clients[req.ClientId]
	if !exists {
		room.mutex.Unlock()
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "Клиент не присоединился к комнате",
		}, nil
	}

	// Сохраняем публичный ключ
	client.PublicKey = req.PublicKey
	room.mutex.Unlock()

	// Сохранение публичного ключа в RedisStore
	err := s.redisStore.SavePublicKey(ctx, req.RoomId, req.ClientId, req.PublicKey)
	if err != nil {
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "Не удалось сохранить публичный ключ",
		}, nil
	}

	// Публикация публичного ключа в exchange комнаты
	err = s.rabbitMQChannel.Publish(
		req.RoomId, // имя exchange
		"",         // routing key (не используется в fanout)
		false,
		false,
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        []byte(req.PublicKey),
			Headers: amqp.Table{
				"type":      "public_key",
				"sender_id": req.ClientId,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("Не удалось опубликовать публичный ключ: %v", err)
	}

	// Проверка, все ли клиенты присоединились и отправили свои публичные ключи
	publicKeys, err := s.redisStore.GetPublicKeys(ctx, req.RoomId)
	if err != nil {
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "Не удалось получить публичные ключи",
		}, nil
	}

	s.roomsMutex.RLock()
	totalClients := len(s.rooms[req.RoomId].Clients)
	s.roomsMutex.RUnlock()

	if len(publicKeys) >= totalClients {
		// Все публичные ключи получены, можно уведомить клиентов или выполнить дополнительные действия
		log.Printf("Все публичные ключи в комнате %s получены", req.RoomId)
		// Например, можно отправить уведомление через RabbitMQ или другой механизм
	}

	return &chatpb.SendPublicKeyResponse{
		Success: true,
	}, nil
}

// Метод отправки сообщения
func (s *ChatServer) SendMessage(ctx context.Context, req *chatpb.SendMessageRequest) (*chatpb.SendMessageResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return &chatpb.SendMessageResponse{
			Success: false,
			Error:   "Комната не найдена",
		}, nil
	}

	// Публикация обычного сообщения в exchange комнаты
	err := s.rabbitMQChannel.Publish(
		room.RoomID, // имя exchange
		"",          // routing key (не используется в fanout)
		false,
		false,
		amqp.Publishing{
			ContentType: "application/octet-stream",
			Body:        req.EncryptedMessage,
			Headers: amqp.Table{
				"type":      "message",
				"sender_id": req.ClientId,
			},
		},
	)
	if err != nil {
		return &chatpb.SendMessageResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &chatpb.SendMessageResponse{
		Success: true,
	}, nil
}

// Метод получения сообщений
func (s *ChatServer) ReceiveMessages(req *chatpb.ReceiveMessagesRequest, stream chatpb.ChatService_ReceiveMessagesServer) error {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return fmt.Errorf("Комната не найдена")
	}

	// Создание уникального имени очереди для клиента
	queueName := "queue_" + req.ClientId + "_" + uuid.New().String()

	// Объявление очереди
	q, err := s.rabbitMQChannel.QueueDeclare(
		queueName, // имя очереди
		false,     // durable
		true,      // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf("Не удалось объявить очередь: %v", err)
	}

	// Привязка очереди к exchange комнаты
	err = s.rabbitMQChannel.QueueBind(
		q.Name,      // имя очереди
		"",          // routing key (не используется в fanout)
		room.RoomID, // имя exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("Не удалось привязать очередь: %v", err)
	}

	// Подписка на очередь
	msgs, err := s.rabbitMQChannel.Consume(
		q.Name, // имя очереди
		"",     // consumer tag
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("Не удалось подписаться на сообщения: %v", err)
	}

	// Горутина для прослушивания сообщений и отправки клиенту
	done := make(chan bool)

	go func() {
		for msg := range msgs {
			msgType, _ := msg.Headers["type"].(string)
			senderID, _ := msg.Headers["sender_id"].(string)

			if senderID == req.ClientId {
				// Игнорируем сообщения от самого себя
				continue
			}

			var response *chatpb.ReceiveMessagesResponse

			if msgType == "public_key" {
				response = &chatpb.ReceiveMessagesResponse{
					Type:             "public_key",
					SenderId:         senderID,
					EncryptedMessage: msg.Body, // Публичный ключ в теле сообщения
				}
			} else if msgType == "message" {
				response = &chatpb.ReceiveMessagesResponse{
					Type:             "message",
					SenderId:         senderID,
					EncryptedMessage: msg.Body,
				}
			} else {
				// Неизвестный тип сообщения
				continue
			}

			if err := stream.Send(response); err != nil {
				log.Printf("Не удалось отправить сообщение клиенту %s: %v", req.ClientId, err)
				done <- true
				return
			}

			log.Printf("Сообщение отправлено клиенту %s", req.ClientId)
		}
	}()

	<-done
	return nil
}

// Метод получения публичных ключей
func (s *ChatServer) GetPublicKeys(ctx context.Context, req *chatpb.GetPublicKeysRequest) (*chatpb.GetPublicKeysResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Комната с ID %s не найдена", req.RoomId)
	}

	room.mutex.RLock()
	defer room.mutex.RUnlock()

	var publicKeys []*chatpb.ClientPublicKey
	for clientID, client := range room.Clients {
		if client.PublicKey != "" {
			publicKeys = append(publicKeys, &chatpb.ClientPublicKey{
				ClientId:  clientID,
				PublicKey: client.PublicKey,
			})
		}
	}

	return &chatpb.GetPublicKeysResponse{
		PublicKeys: publicKeys,
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Не удалось прослушивать порт: %v", err)
	}

	grpcServer := grpc.NewServer()
	chatServer := NewChatServer()

	chatpb.RegisterChatServiceServer(grpcServer, chatServer)

	log.Println("Сервер запущен на порту :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}
