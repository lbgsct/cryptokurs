// web/handlers/auth.go
package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

var JwtKey = []byte("your_secret_key") // Замените на надежный ключ

type Credentials struct {
	Username string `form:"username" json:"username" binding:"required"`
	Password string `form:"password" json:"password" binding:"required"`
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

var users = make(map[string]string) // Временное хранилище пользователей

// Register регистрирует нового пользователя
func Register(c *gin.Context) {
	var creds Credentials
	if err := c.ShouldBind(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Проверка, существует ли пользователь
	if _, exists := users[creds.Username]; exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User already exists"})
		return
	}

	// Хеширование пароля
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not hash password"})
		return
	}

	// Сохранение пользователя в хранилище (замените на базу данных в реальном приложении)
	users[creds.Username] = string(hashedPassword)

	c.JSON(http.StatusOK, gin.H{"message": "User registered successfully"})
}

// Login аутентифицирует пользователя и возвращает JWT
func Login(c *gin.Context) {
	var creds Credentials
	if err := c.ShouldBind(&creds); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// Проверяем наличие пользователя
	storedPassword, ok := users[creds.Username]
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not found"})
		return
	}

	// Проверяем пароль
	if err := bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(creds.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Incorrect password"})
		return
	}

	// Создаем токен JWT
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(JwtKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not create token"})
		return
	}

	// Устанавливаем токен в cookie
	c.SetCookie("token", tokenString, int(expirationTime.Sub(time.Now()).Seconds()), "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "Logged in successfully"})
}
