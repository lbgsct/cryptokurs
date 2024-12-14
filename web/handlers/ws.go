package handlers

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	chatpb "github.com/lbgsct/cryptokurs/proto/chatpb"
	"google.golang.org/grpc"
)

// Объявление upgrader для WebSocket
var upgraderWS = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // В реальном приложении ограничьте происхождение
	},
}

var grpcClientWS chatpb.ChatServiceClient

func init() {
	// Устанавливаем соединение с gRPC-сервером
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Не удалось подключиться к gRPC-серверу: %v", err)
	}
	grpcClientWS = chatpb.NewChatServiceClient(conn)
}

// clients хранит активные WebSocket-соединения по username
var clients = make(map[string]*websocket.Conn)
var clientsMutex sync.RWMutex

// SendNotification отправляет уведомление пользователю по его username
func SendNotification(username string, notification map[string]interface{}) error {
	clientsMutex.RLock()
	conn, exists := clients[username]
	clientsMutex.RUnlock()
	if !exists {
		log.Printf("Нет активного WebSocket-соединения для пользователя %s", username)
		return nil // Не возвращаем ошибку, если соединение не активно
	}

	err := conn.WriteJSON(map[string]interface{}{
		"type":         "notification",
		"notification": notification,
	})
	if err != nil {
		log.Printf("Ошибка при отправке уведомления пользователю %s: %v", username, err)
		return err
	}

	return nil
}

// WebSocketHandler обрабатывает WebSocket соединения
func WebSocketHandler(c *gin.Context) {
	conn, err := upgraderWS.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Ошибка при обновлении соединения:", err)
		return
	}
	defer conn.Close()

	username := getCurrentUsername(c)
	if username == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("Необходимо войти в систему"))
		return
	}

	clientsMutex.Lock()
	clients[username] = conn
	clientsMutex.Unlock()
	defer func() {
		clientsMutex.Lock()
		delete(clients, username)
		clientsMutex.Unlock()
	}()

	log.Printf("Пользователь %s подключился к WebSocket\n", username)

	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Ошибка при чтении сообщения:", err)
			break
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			log.Println("Сообщение без типа")
			continue
		}

		switch msgType {
		case "chat":
			content, ok1 := msg["content"].(string)
			roomID, ok2 := msg["room_id"].(string)
			if !ok1 || !ok2 || content == "" || roomID == "" {
				log.Println("Некорректное содержимое сообщения")
				continue
			}

			cipherContext := LoadCipherContext(roomID, username)
			if cipherContext == nil {
				conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Шифровальный контекст не инициализирован",
				})
				continue
			}

			encryptedMessage, err := cipherContext.Encrypt([]byte(content))
			if err != nil {
				log.Printf("Ошибка при шифровании сообщения: %v", err)
				conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Ошибка шифрования сообщения",
				})
				continue
			}

			_, err = grpcClientWS.SendMessage(context.Background(), &chatpb.SendMessageRequest{
				RoomId:           roomID,
				ClientId:         username,
				EncryptedMessage: encryptedMessage,
			})
			if err != nil {
				log.Printf("Ошибка при отправке сообщения через gRPC: %v", err)
				conn.WriteJSON(map[string]interface{}{
					"type":    "error",
					"message": "Ошибка отправки сообщения",
				})
				continue
			}

			err = conn.WriteJSON(map[string]interface{}{
				"type":      "chat",
				"sender":    username,
				"content":   content,
				"room_id":   roomID,
				"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			})
			if err != nil {
				log.Printf("Ошибка при отправке сообщения обратно: %v", err)
				break
			}

		default:
			log.Printf("Неизвестный тип сообщения: %s", msgType)
		}
	}
}

// getCurrentUsername извлекает имя пользователя из контекста
func getCurrentUsername(c *gin.Context) string {
	usernameVal, exists := c.Get("username")
	if !exists {
		return ""
	}
	username, ok := usernameVal.(string)
	if !ok {
		return ""
	}
	return username
}
