package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Types de messages échangés
const (
	MsgTypeText    = "text"
	MsgTypeImage   = "image"
	MsgTypeAudio   = "audio"
	MsgTypeVideo   = "video"
	MsgTypeGif     = "gif"
	MsgTypeJoin    = "join"
	MsgTypeLeave   = "leave"
	MsgTypeMembers = "members"
)

// Message représente un message WebSocket
type Message struct {
	Type      string `json:"type"`
	GroupID   string `json:"group_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Avatar    string `json:"avatar"`
	Content   string `json:"content"`            // texte ou URL média
	Duration  int    `json:"duration,omitempty"` // secondes (audio/vidéo)
	Timestamp int64  `json:"timestamp"`
}

// Client représente une connexion WebSocket active
type Client struct {
	ID       string
	UserID   string
	Username string
	Avatar   string
	GroupID  string
	conn     *websocket.Conn
	send     chan []byte
	hub      *Hub
}

// Hub gère toutes les connexions et le broadcast par groupe
type Hub struct {
	mu      sync.RWMutex
	// groups : groupID → map[clientID]*Client
	groups  map[string]map[string]*Client
	// register/unregister sont des canaux pour éviter les race conditions
	register   chan *Client
	unregister chan *Client
	broadcast  chan *groupMessage
}

type groupMessage struct {
	groupID string
	data    []byte
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// En production : vérifier l'origine
		return true
	},
}

func NewHub() *Hub {
	return &Hub{
		groups:     make(map[string]map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *groupMessage, 256),
	}
}

// Run démarre la boucle principale du Hub (à lancer en goroutine)
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.groups[client.GroupID] == nil {
				h.groups[client.GroupID] = make(map[string]*Client)
			}
			h.groups[client.GroupID][client.ID] = client
			h.mu.Unlock()

			// Notifier le groupe qu'un membre a rejoint
			h.broadcastToGroup(client.GroupID, &Message{
				Type:      MsgTypeJoin,
				GroupID:   client.GroupID,
				UserID:    client.UserID,
				Username:  client.Username,
				Avatar:    client.Avatar,
				Timestamp: time.Now().Unix(),
			})
			log.Printf("👤 %s a rejoint le groupe %s", client.Username, client.GroupID)

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.groups[client.GroupID]; ok {
				delete(clients, client.ID)
				if len(clients) == 0 {
					delete(h.groups, client.GroupID)
				}
			}
			h.mu.Unlock()
			close(client.send)

			// Notifier le groupe
			h.broadcastToGroup(client.GroupID, &Message{
				Type:      MsgTypeLeave,
				GroupID:   client.GroupID,
				UserID:    client.UserID,
				Username:  client.Username,
				Timestamp: time.Now().Unix(),
			})
			log.Printf("👋 %s a quitté le groupe %s", client.Username, client.GroupID)

		case msg := <-h.broadcast:
			h.mu.RLock()
			clients := h.groups[msg.groupID]
			h.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.send <- msg.data:
				default:
					// Le canal est plein, on déconnecte le client
					close(client.send)
					h.mu.Lock()
					delete(h.groups[msg.groupID], client.ID)
					h.mu.Unlock()
				}
			}
		}
	}
}

func (h *Hub) broadcastToGroup(groupID string, msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Erreur marshal message : %v", err)
		return
	}
	h.broadcast <- &groupMessage{groupID: groupID, data: data}
}

// ServeWS gère une nouvelle connexion WebSocket
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, userID, username, avatar, groupID, clientID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Erreur upgrade WebSocket : %v", err)
		return
	}

	client := &Client{
		ID:       clientID,
		UserID:   userID,
		Username: username,
		Avatar:   avatar,
		GroupID:  groupID,
		conn:     conn,
		send:     make(chan []byte, 64),
		hub:      h,
	}

	h.register <- client

	// Lancer les goroutines de lecture/écriture
	go client.writePump()
	go client.readPump()
}

// readPump lit les messages entrants du client
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512 * 1024) // 512 KB max par message WS
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Erreur WebSocket : %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("Message invalide de %s : %v", c.Username, err)
			continue
		}

		// Forcer les métadonnées depuis le JWT (pas depuis le client)
		msg.UserID = c.UserID
		msg.Username = c.Username
		msg.Avatar = c.Avatar
		msg.GroupID = c.GroupID
		msg.Timestamp = time.Now().Unix()

		c.hub.broadcastToGroup(c.GroupID, &msg)
	}
}

// writePump envoie les messages sortants au client
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}

		case <-ticker.C:
			// Ping keepalive
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// GetGroupMembers retourne les membres connectés d'un groupe
func (h *Hub) GetGroupMembers(groupID string) []map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var members []map[string]string
	for _, client := range h.groups[groupID] {
		members = append(members, map[string]string{
			"user_id":  client.UserID,
			"username": client.Username,
			"avatar":   client.Avatar,
		})
	}
	return members
}
