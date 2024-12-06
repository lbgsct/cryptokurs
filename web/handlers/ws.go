// web/handlers/ws.go
package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	chatpb "github.com/lbgsct/cryptokurs/proto/chatpb"
	"google.golang.org/grpc"
)

// Объявление upgrader для WebSocket
var upgrader = websocket.Upgrader{
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

// WebSocketHandler обрабатывает WebSocket соединения
func WebSocketHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("Ошибка при обновлении соединения:", err)
		return
	}
	defer conn.Close()

	// Получение room_id из параметров запроса
	roomID := c.Query("room_id")
	username, exists := c.Get("username") // Получаем имя пользователя из контекста
	if !exists {
		conn.WriteMessage(websocket.TextMessage, []byte("Необходимо войти в систему"))
		return
	}

	if roomID == "" || username == "" {
		conn.WriteMessage(websocket.TextMessage, []byte("room_id и username обязательны"))
		return
	}

	log.Printf("Пользователь %s подключился к комнате %s\n", username.(string), roomID)

	// Основной цикл обработки сообщений
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Ошибка при чтении сообщения:", err)
			break
		}

		// Здесь можно добавить логику шифрования сообщения с использованием вашего криптографического контекста

		// Отправляем сообщение на gRPC-сервер
		_, err = grpcClientWS.SendMessage(c, &chatpb.SendMessageRequest{
			RoomId:           roomID,
			ClientId:         username.(string),
			EncryptedMessage: message, // Замените на зашифрованное сообщение
		})
		if err != nil {
			log.Println("Ошибка при отправке сообщения через gRPC:", err)
			continue
		}

		// Рассылаем сообщение всем клиентам в комнате (реализация зависит от вашего gRPC-сервера)
		// В данном примере мы просто эмулируем рассылку обратно отправителю
		err = conn.WriteMessage(messageType, message)
		if err != nil {
			log.Println("Ошибка при отправке сообщения обратно:", err)
			break
		}
	}
}
