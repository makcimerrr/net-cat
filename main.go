package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type client struct {
	conn     net.Conn
	username string
	writer   *bufio.Writer
}

var (
	clients       []*client
	clientsMu     sync.Mutex
	chatHistory   []string
	activeClients int
	maxClients    = 10
)

func main() {
	port := 8989
	if len(os.Args) > 1 {
		p, err := strconv.Atoi(os.Args[1])
		if err != nil {
			log.Fatalf("[USAGE]: %s $port", os.Args[0])
		}
		port = p
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}
	defer listener.Close()

	log.Printf("Server started, listening on port %d", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Failed to accept connection:", err)
			continue
		}

		if activeClients >= maxClients {
			conn.Write([]byte("Maximum number of clients reached. Please try again later.\n"))
			conn.Close()
			continue
		}

		activeClients++
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	sendWelcomeMessage(conn)

	username := getUsername(conn)
	client := &client{
		conn:     conn,
		username: username,
		writer:   bufio.NewWriter(conn),
	}

	clientsMu.Lock()
	clients = append(clients, client)
	clientsMu.Unlock()

	notifyJoin(client)

	sendChatHistory(client)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		message := scanner.Text()

		if strings.ToLower(message) == "/quit" {
			break
		}

		sendMessage(client, message)
	}

	conn.Close()

	clientsMu.Lock()
	removeClient(client)
	clientsMu.Unlock()

	notifyLeave(client)

	activeClients--
}

func getUsername(conn net.Conn) string {
	conn.Write([]byte("[ENTER YOUR USERNAME]: "))

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		return scanner.Text()
	}

	return "unknown"
}

func notifyJoin(c *client) {
	message := fmt.Sprintf("%s has joined the chat\n", c.username)
	broadcastMessage(c, message)
	clientsMu.Lock()
	chatHistory = append(chatHistory, message)
	clientsMu.Unlock()
}

func notifyLeave(c *client) {
	message := fmt.Sprintf("%s has left the chat\n", c.username)
	broadcastMessage(nil, message)
	clientsMu.Lock()
	chatHistory = append(chatHistory, message)
	clientsMu.Unlock()
}

func sendMessage(sender *client, message string) {
	if strings.TrimSpace(message) == "" {
		return // Ignore empty messages
	}

	msg := fmt.Sprintf("[%s][%s]: %s\n", getTimeStamp(), sender.username, message)
	broadcastMessage(sender, msg)

	clientsMu.Lock()
	chatHistory = append(chatHistory, msg)
	clientsMu.Unlock()
}

func sendWelcomeMessage(conn net.Conn) {
	file, err := os.Open("Welcome.txt")
	if err != nil {
		log.Println("Failed to open welcome file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		message := scanner.Text()
		_, err := conn.Write([]byte(message + "\n"))
		if err != nil {
			log.Println("Error sending welcome message:", err)
			return
		}
	}

	if err := scanner.Err(); err != nil {
		log.Println("Error reading welcome file:", err)
		return
	}
}

func broadcastMessage(sender *client, message string) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for _, c := range clients {
		if sender == nil || c != sender {
			c.writer.WriteString(message)
			c.writer.Flush()
		}
	}
}

func sendChatHistory(c *client) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	for _, msg := range chatHistory {
		c.writer.WriteString(msg)
		c.writer.Flush()
	}

	for _, client := range clients {
		if client != c {
			message := fmt.Sprintf("[%s] %s has joined the chat\n", getTimeStamp(), client.username)
			c.writer.WriteString(message)
			c.writer.Flush()
		}
	}
}

func removeClient(c *client) {
	for i, client := range clients {
		if client == c {
			clients = append(clients[:i], clients[i+1:]...)
			break
		}
	}
}

func getTimeStamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
