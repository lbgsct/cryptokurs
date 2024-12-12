Сначала запускается docker:
docker compose up

Если в DataGrip нет таблиц user и messanges, создаем их
CREATE TABLE messages (
    id SERIAL PRIMARY KEY,
    room_id TEXT NOT NULL,
    sender_id TEXT NOT NULL,
    encrypted_message BYTEA NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
Потом переходим в вебку
cd web
Там запускаем через go run .
Переходим на localhost:8080