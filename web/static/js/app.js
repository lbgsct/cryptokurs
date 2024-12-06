// web/static/js/app.js

document.addEventListener('DOMContentLoaded', () => {
    const registerForm = document.getElementById('register-form');
    const loginForm = document.getElementById('login-form');
    const createChatForm = document.getElementById('create-chat-form');
    const joinChatForm = document.getElementById('join-chat-form');
    const sendMessageButton = document.getElementById('send-message-button');
    const messageInput = document.getElementById('message');
    const messagesDiv = document.getElementById('messages');
    const logoutButton = document.getElementById('logout-button');

    let socket = null;

    // Функция для получения параметра из URL
    function getQueryParam(param) {
        const urlParams = new URLSearchParams(window.location.search);
        return urlParams.get(param);
    }

    const roomID = getQueryParam('room_id');

    if (roomID) {
        connectWebSocket(roomID);
    }

    // Обработка регистрации
    if (registerForm) {
        registerForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const formData = new FormData(registerForm);
            const response = await fetch('/register', {
                method: 'POST',
                body: formData
            });
            const result = await response.json();
            const messageDiv = document.getElementById('message');
            if (response.redirected) {
                window.location.href = response.url;
            } else {
                if (response.ok) {
                    messageDiv.innerHTML = `<p style="color: green;">${result.message}</p>`;
                    registerForm.reset();
                } else {
                    messageDiv.innerHTML = `<p style="color: red;">${result.error}</p>`;
                }
            }
        });
    }

    // Обработка входа
    if (loginForm) {
        loginForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const formData = new FormData(loginForm);
            const response = await fetch('/login', {
                method: 'POST',
                body: formData
            });
            if (response.redirected) {
                window.location.href = response.url;
            } else {
                const result = await response.json();
                const messageDiv = document.getElementById('message');
                if (response.ok) {
                    window.location.href = '/chats';
                } else {
                    messageDiv.innerHTML = `<p style="color: red;">${result.error}</p>`;
                }
            }
        });
    }

    // Обработка создания комнаты
    if (createChatForm) {
        createChatForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const formData = new FormData(createChatForm);
            const response = await fetch('/chats/create_chat', {
                method: 'POST',
                body: formData
            });
            if (response.redirected) {
                window.location.href = response.url;
            } else {
                const result = await response.json();
                if (response.ok) {
                    alert(`Комната создана с ID: ${result.room_id}`);
                    // Перенаправление на страницу чатов с room_id
                    window.location.href = `/chats/?room_id=${result.room_id}`;
                } else {
                    alert(`Ошибка: ${result.error}`);
                }
            }
        });
    }

    // Обработка присоединения к комнате
    if (joinChatForm) {
        joinChatForm.addEventListener('submit', async (e) => {
            e.preventDefault();
            const formData = new FormData(joinChatForm);
            const response = await fetch('/chats/join_chat', {
                method: 'POST',
                body: formData
            });
            if (response.redirected) {
                window.location.href = response.url;
            } else {
                const result = await response.json();
                if (response.ok) {
                    alert(`Присоединились к комнате с ID: ${result.room_id}`);
                    // Перенаправление на страницу чатов с room_id
                    window.location.href = `/chats/?room_id=${result.room_id}`;
                } else {
                    alert(`Ошибка: ${result.error}`);
                }
            }
        });
    }

    // Обработка отправки сообщения через WebSocket
    if (sendMessageButton) {
        sendMessageButton.addEventListener('click', () => {
            const message = messageInput.value.trim();
            if (message && socket && socket.readyState === WebSocket.OPEN) {
                socket.send(message);
                appendMessage("Вы", message);
                messageInput.value = '';
            }
        });
    }

    // Обработка выхода из системы
    if (logoutButton) {
        logoutButton.addEventListener('click', async () => {
            const response = await fetch('/logout', {
                method: 'POST'
            });
            if (response.redirected) {
                window.location.href = response.url;
            } else {
                const result = await response.json();
                if (response.ok) {
                    window.location.href = '/';
                } else {
                    alert('Ошибка при выходе из системы');
                }
            }
        });
    }

    // Функция подключения к WebSocket
    function connectWebSocket(roomID) {
        socket = new WebSocket(`ws://${window.location.host}/chats/ws?room_id=${roomID}`);

        socket.onopen = function(event) {
            console.log("WebSocket соединение установлено");
        };

        socket.onmessage = function(event) {
            const message = event.data;
            appendMessage("Другой пользователь", message);
        };

        socket.onclose = function(event) {
            console.log("WebSocket соединение закрыто");
        };

        socket.onerror = function(error) {
            console.error("WebSocket ошибка:", error);
        };
    }

    // Функция добавления сообщения в раздел сообщений
    function appendMessage(sender, message) {
        const messageElement = document.createElement('p');
        messageElement.textContent = `${sender}: ${message}`;
        messagesDiv.appendChild(messageElement);
    }
});
