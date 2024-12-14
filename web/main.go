package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/lbgsct/cryptokurs/web/grpcclient"
	"github.com/lbgsct/cryptokurs/web/handlers"
	"github.com/lbgsct/cryptokurs/web/middleware"
)

func main() {
	grpcclient.InitGRPCClient()
	defer grpcclient.CloseGRPC()
	// Получаем DSN из переменных окружения или используем значение по умолчанию
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		// На случай запуска локально без docker-compose:
		dsn = "postgres://postgres:mysecretpassword@localhost:5432/mydb?sslmode=disable"
	}

	// Инициализируем БД до запуска роутера
	if err := handlers.InitializeDB(dsn); err != nil {
		log.Fatalf("Не удалось инициализировать БД: %v", err)
	}

	router := gin.Default()

	// Загрузка HTML-шаблонов (убедитесь, что путь корректен)
	router.LoadHTMLGlob("templates/*")

	// Обслуживание статических файлов (убедитесь, что путь корректен)
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

	// Группа маршрутов, требующих авторизации
	/*authorized := router.Group("/chats")
	authorized.Use(middleware.AuthMiddleware())
	{
		// Маршрут для меню чатов
		authorized.GET("/menu", handlers.MenuHandler)

		// Маршрут для отображения страницы чата с room_id
		authorized.GET("/chat", handlers.ChatHandler) // /chats/chat?room_id=...

		// Маршрут для отображения страницы создания чата (GET)
		authorized.GET("/create_chat", handlers.ShowCreateChatPage)

		// Маршрут для обработки создания чата (POST)
		authorized.POST("/create_chat", handlers.CreateChat)

		// Маршрут для присоединения к чату (POST)
		authorized.POST("/join_chat", handlers.JoinChat)

		// WebSocket маршрут
		authorized.GET("/ws", handlers.WebSocketHandler)

		// Маршрут для отправки приглашения (POST)
		authorized.POST("/send_invitation", handlers.SendInvitationHandler)

		// Маршрут для получения списка приглашений (GET)
		authorized.GET("/invitations", handlers.ListInvitationsHandler)

		// Маршрут для ответа на приглашение (POST)
		authorized.POST("/respond_invitation", handlers.RespondInvitationHandler)

		// Маршрут для выхода из профиля
		authorized.GET("/logout", handlers.LogoutHandler)
	}*/

	authorized := router.Group("/messenger")
	authorized.Use(middleware.AuthMiddleware())
	{
		// Маршрут для меню чатов
		authorized.GET("/lobby", handlers.LobbyHandler)

		// Маршрут для отображения страницы чата с room_id
		authorized.GET("/chat", handlers.ChatHandler) // /chats/chat?room_id=...

		// Маршрут для отображения страницы создания чата (GET)
		authorized.GET("/create_chat", handlers.ShowCreateChatPage)

		// Маршрут для обработки создания чата (POST)
		authorized.POST("/create_chat", handlers.CreateChat)

		// Маршрут для присоединения к чату (POST)
		authorized.POST("/join_chat", handlers.JoinChat)

		// WebSocket маршрут
		authorized.GET("/ws", handlers.WebSocketHandler)

		// Удаляем маршруты отправки приглашений через меню
		// authorized.POST("/send_invitation", handlers.SendInvitationHandler)
		// authorized.GET("/invitations", handlers.ListInvitationsHandler)
		// authorized.POST("/respond_invitation", handlers.RespondInvitationHandler)

		// Если осталась необходимость обрабатывать приглашения, но через другие формы, оставьте эти маршруты
		authorized.POST("/send_invitation", handlers.SendInvitationHandler)
		authorized.GET("/invitations", handlers.ListInvitationsHandler)
		authorized.POST("/respond_invitation", handlers.RespondInvitationHandler)

		// Маршрут для выхода из профиля
		authorized.GET("/logout", handlers.LogoutHandler)
	}

	// Запуск сервера на порту 8080
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Не удалось запустить сервер: %v", err)
	}
}
