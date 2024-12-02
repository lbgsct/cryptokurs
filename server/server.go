package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"net"
	"sync"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatpb "github.com/lbgsct/cryptokurs/proto/chatpb"
)

type ChatServer struct {
	chatpb.UnimplementedChatServiceServer
	redisStore      *RedisStore
	rooms           map[string]*ChatRoom
	roomsMutex      sync.RWMutex
	rabbitMQConn    *amqp.Connection
	rabbitMQChannel *amqp.Channel
}

type ChatRoom struct {
	RoomID    string
	Algorithm string
	Mode      string
	Padding   string
	Prime     string
	Clients   map[string]*Client
	mutex     sync.RWMutex
}

type Client struct {
	ClientID   string
	PrivateKey *big.Int
	PublicKey  *big.Int
	SharedKey  []byte
}

func NewChatServer() *ChatServer {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
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
	bytes := make([]byte, 4) // 4 bytes will give us an 8-character hex string
	_, err := rand.Read(bytes)
	if err != nil {
		log.Fatalf("Failed to generate random bytes: %v", err)
	}
	return hex.EncodeToString(bytes)
}

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
		return nil, fmt.Errorf("failed to create room in Redis: %v", err)
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
		return nil, fmt.Errorf("failed to declare exchange: %v", err)
	}

	return &chatpb.CreateRoomResponse{
		RoomId: roomID,
	}, nil
}

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

func (s *ChatServer) CloseRoom(ctx context.Context, req *chatpb.CloseRoomRequest) (*chatpb.CloseRoomResponse, error) {
	s.roomsMutex.Lock()
	defer s.roomsMutex.Unlock()

	room, exists := s.rooms[req.RoomId]
	if !exists {
		return &chatpb.CloseRoomResponse{
			Success: false,
		}, fmt.Errorf("room not found")
	}

	// Delete the queue from RabbitMQ
	_, err := s.rabbitMQChannel.QueueDelete(
		room.RoomID,
		false,
		false,
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to delete queue: %v", err)
	}

	delete(s.rooms, req.RoomId)

	return &chatpb.CloseRoomResponse{
		Success: true,
	}, nil
}

func (s *ChatServer) JoinRoom(ctx context.Context, req *chatpb.JoinRoomRequest) (*chatpb.JoinRoomResponse, error) {
	s.roomsMutex.RLock()
	exists, err := s.redisStore.RoomExists(ctx, req.RoomId)
	s.roomsMutex.RUnlock()
	if err != nil || !exists {
		return &chatpb.JoinRoomResponse{
			Success: false,
			Error:   "room not found",
		}, nil
	}

	err = s.redisStore.AddClientToRoom(ctx, req.RoomId, req.ClientId)
	if err != nil {
		return &chatpb.JoinRoomResponse{
			Success: false,
			Error:   "failed to add client to room",
		}, nil
	}

	return &chatpb.JoinRoomResponse{
		Success: true,
	}, nil
}

func (s *ChatServer) LeaveRoom(ctx context.Context, req *chatpb.LeaveRoomRequest) (*chatpb.LeaveRoomResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return &chatpb.LeaveRoomResponse{
			Success: false,
			Error:   "room not found",
		}, nil
	}

	room.mutex.Lock()
	defer room.mutex.Unlock()

	if _, exists := room.Clients[req.ClientId]; !exists {
		return &chatpb.LeaveRoomResponse{
			Success: false,
			Error:   "client not found in room",
		}, nil
	}

	delete(room.Clients, req.ClientId)

	return &chatpb.LeaveRoomResponse{
		Success: true,
	}, nil
}

func (s *ChatServer) SendPublicKey(ctx context.Context, req *chatpb.SendPublicKeyRequest) (*chatpb.SendPublicKeyResponse, error) {
	s.roomsMutex.RLock()
	exists, err := s.redisStore.RoomExists(ctx, req.RoomId)
	s.roomsMutex.RUnlock()
	if err != nil || !exists {
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "room not found",
		}, nil
	}

	err = s.redisStore.SavePublicKey(ctx, req.RoomId, req.ClientId, req.PublicKey)
	if err != nil {
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "failed to save public key",
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
		return nil, fmt.Errorf("failed to publish public key: %v", err)
	}

	publicKeys, err := s.redisStore.GetPublicKeys(ctx, req.RoomId)
	if err != nil {
		return &chatpb.SendPublicKeyResponse{
			Success: false,
			Error:   "failed to retrieve public keys",
		}, nil
	}

	if len(publicKeys) >= 2 {
		// Публичные ключи всех клиентов получены
	}

	return &chatpb.SendPublicKeyResponse{
		Success: true,
	}, nil
}

func (s *ChatServer) SendMessage(ctx context.Context, req *chatpb.SendMessageRequest) (*chatpb.SendMessageResponse, error) {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return &chatpb.SendMessageResponse{
			Success: false,
			Error:   "room not found",
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

func (s *ChatServer) ReceiveMessages(req *chatpb.ReceiveMessagesRequest, stream chatpb.ChatService_ReceiveMessagesServer) error {
	s.roomsMutex.RLock()
	room, exists := s.rooms[req.RoomId]
	s.roomsMutex.RUnlock()
	if !exists {
		return fmt.Errorf("room not found")
	}

	// Создайте уникальное имя очереди для клиента
	queueName := "queue_" + req.ClientId

	// Объявите очередь
	q, err := s.rabbitMQChannel.QueueDeclare(
		queueName, // имя очереди
		false,     // durable
		false,     // delete when unused
		true,      // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %v", err)
	}

	// Привяжите очередь к exchange комнаты
	err = s.rabbitMQChannel.QueueBind(
		q.Name,      // имя очереди
		"",          // routing key (не используется в fanout)
		room.RoomID, // имя exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind queue: %v", err)
	}

	// Подпишитесь на очередь
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
		return fmt.Errorf("failed to consume messages: %v", err)
	}

	for msg := range msgs {
		msgType := ""
		if val, ok := msg.Headers["type"]; ok {
			msgType, _ = val.(string)
		}
		senderID := ""
		if val, ok := msg.Headers["sender_id"]; ok {
			senderID, _ = val.(string)
		}
		if senderID == req.ClientId {
			log.Printf("Message from client %s to itself ignored", senderID)
			continue // Пропускаем сообщения от самого себя
		}

		log.Printf("Received %s from client %s to room %s", msgType, senderID, req.RoomId)

		resp := &chatpb.ReceiveMessagesResponse{
			Type:             msgType,
			EncryptedMessage: msg.Body,
			SenderId:         senderID,
		}

		if err := stream.Send(resp); err != nil {
			log.Printf("Failed to send message to client %s: %v", req.ClientId, err)
			return err
		}

		log.Printf("Message sent to client %s", req.ClientId)
	}

	return nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	chatServer := NewChatServer()

	chatpb.RegisterChatServiceServer(grpcServer, chatServer)

	log.Println("Server is running on port :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
