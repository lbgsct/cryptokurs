// web/handlers/chat.go
package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	chatpb "github.com/lbgsct/cryptokurs/proto/chatpb"
	"google.golang.org/grpc"
)

var grpcClient chatpb.ChatServiceClient

func init() {
	// Устанавливаем соединение с gRPC-сервером
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		panic(fmt.Sprintf("Не удалось подключиться к gRPC-серверу: %v", err))
	}
	grpcClient = chatpb.NewChatServiceClient(conn)
}

// CreateChat обрабатывает создание новой комнаты чата
func CreateChat(c *gin.Context) {
	algorithm := c.PostForm("algorithm")
	mode := c.PostForm("mode")
	padding := c.PostForm("padding")

	// Получаем имя пользователя из контекста (из JWT)
	_, exists := c.Get("username")
	if !exists {
		c.HTML(http.StatusUnauthorized, "chat.html", gin.H{"error": "Необходимо войти в систему"})
		return
	}

	// Вызываем метод создания комнаты на gRPC-сервере
	resp, err := grpcClient.CreateRoom(context.Background(), &chatpb.CreateRoomRequest{
		Algorithm: algorithm,
		Mode:      mode,
		Padding:   padding,
		// Добавьте другие необходимые поля, например, Prime
	})
	if err != nil {
		c.HTML(http.StatusInternalServerError, "chat.html", gin.H{"error": "Не удалось создать комнату"})
		return
	}

	// Перенаправление на страницу чатов с room_id
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/chats/?room_id=%s", resp.RoomId))
}

// JoinChat обрабатывает присоединение к существующей комнате чата
func JoinChat(c *gin.Context) {
	roomID := c.PostForm("room_id")
	username, exists := c.Get("username")
	if !exists {
		c.HTML(http.StatusUnauthorized, "chat.html", gin.H{"error": "Необходимо войти в систему"})
		return
	}

	// Вызываем метод присоединения к комнате на gRPC-сервере
	resp, err := grpcClient.JoinRoom(context.Background(), &chatpb.JoinRoomRequest{
		RoomId:   roomID,
		ClientId: username.(string), // Используем имя пользователя в качестве идентификатора клиента
	})
	if err != nil || !resp.Success {
		c.HTML(http.StatusInternalServerError, "chat.html", gin.H{"error": "Не удалось присоединиться к комнате"})
		return
	}

	// Перенаправление на страницу чатов с room_id
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/chats/?room_id=%s", roomID))
}
