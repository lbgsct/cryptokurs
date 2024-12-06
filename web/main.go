// web/main.go
package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/lbgsct/cryptokurs/web/handlers"
	"github.com/lbgsct/cryptokurs/web/middleware"
)

func main() {
	router := gin.Default()

	// Загрузка HTML-шаблонов
	router.LoadHTMLGlob("templates/*")

	// Обслуживание статических файлов
	router.Static("/static", "./static")

	// Главная страница (landing page)
	router.GET("/", func(c *gin.Context) {
		c.HTML(200, "home.html", nil)
	})

	// Маршруты для авторизации
	router.GET("/register", func(c *gin.Context) {
		c.HTML(200, "register.html", nil)
	})
	router.POST("/register", handlers.Register)

	router.GET("/login", func(c *gin.Context) {
		c.HTML(200, "login.html", nil)
	})
	router.POST("/login", handlers.Login)

	// Маршрут выхода
	//router.POST("/logout", handlers.Logout)

	// Группа маршрутов, требующих авторизации
	authorized := router.Group("/chats")
	authorized.Use(middleware.AuthMiddleware())
	{
		// Главная страница чата
		authorized.GET("/", func(c *gin.Context) {
			roomID := c.Query("room_id")
			c.HTML(200, "chat.html", gin.H{
				"RoomID": roomID,
			})
		})

		// Чат-операции
		authorized.POST("/create_chat", handlers.CreateChat)
		authorized.POST("/join_chat", handlers.JoinChat)
		// Отправка сообщений будет обрабатываться через WebSocket, поэтому можно удалить этот маршрут
		// authorized.POST("/send_message", handlers.SendMessage)

		// WebSocket маршрут
		authorized.GET("/ws", handlers.WebSocketHandler)
	}

	// Запуск сервера на порту 8080
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}
