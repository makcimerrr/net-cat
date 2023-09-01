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

	"github.com/atotto/clipboard"
	"github.com/chzyer/readline"
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

	ip, err := getIPv4Address()
	if err != nil {
		log.Fatalf("Failed to get IPv4 address: %v", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatal("Failed to start server:", err)
	}
	defer listener.Close()

	cmd := fmt.Sprintf("nc %s %d", ip, port)

	clipboard.WriteAll(cmd) // Copier la commande dans le presse-papiers

	rl, err := readline.New("> ")
	if err != nil {
		log.Fatal(err)
	}
	defer rl.Close()

	log.Printf("Server started, listening on port %d, paste in terminal to connect", port)

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

func getIPv4Address() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String(), nil
		}
	}

	return "", fmt.Errorf("no ipv4 address found")
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
	conn.Write([]byte("\x1b[35;1;4m[ENTER YOUR USERNAME]: \x1b[0m"))

	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		username := scanner.Text()
		if username == "" {
			conn.Write([]byte("\x1b[31;1m[ERROR]: Username cannot be empty\x1b[0m\n"))
			return getUsername(conn) // Demander à nouveau le nom d'utilisateur
		}
		return username
	}

	return "unknown"
}

func notifyJoin(c *client) {
	message := fmt.Sprintf("\x1b[32;1m%s has joined the chat\x1b[0m\n", c.username)
	broadcastMessage(c, message)
	clientsMu.Lock()
	chatHistory = append(chatHistory, message)
	clientsMu.Unlock()
}

func notifyLeave(c *client) {
	message := fmt.Sprintf("\x1b[31;1m%s has left the chat\x1b[0m\n", c.username)
	broadcastMessage(nil, message)
	clientsMu.Lock()
	chatHistory = append(chatHistory, message)
	clientsMu.Unlock()
}

func sendMessage(sender *client, message string) {
	timeStamp := getTimeStamp() // Obtenir l'horodatage actuel

	if strings.TrimSpace(message) == "" {
		// Si le message est vide, envoyer uniquement l'horodatage et le nom d'utilisateur à l'expéditeur
		msg := fmt.Sprintf("\x1b[36m[%s][%s]:\x1b[0m\n", timeStamp, sender.username)
		sender.writer.WriteString(msg)
		sender.writer.Flush()
		return // Ne pas envoyer de message vide aux autres clients
	}
	msg := fmt.Sprintf("\x1b[36m[%s][%s]: %s\x1b[0m\n", timeStamp, sender.username, message)
	broadcastMessage(sender, msg)

	// Afficher le message avec l'horodatage dans le terminal de l'expéditeur
	sender.writer.WriteString(msg)
	sender.writer.Flush()

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

		// Appliquer des séquences ANSI au message
		formattedMessage := formatWelcomeMessage(message)

		_, err := conn.Write([]byte(formattedMessage + "\n"))
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

func formatWelcomeMessage(message string) string {
	formattedMessage := message

	// Remplacer [orange] et [/orange] par les séquences ANSI pour la couleur orange
	formattedMessage = strings.ReplaceAll(formattedMessage, "[orange]", "\x1b[38;5;208m")
	formattedMessage = strings.ReplaceAll(formattedMessage, "[/orange]", "\x1b[0m\x1b[40;1;37m")

	// Appliquer la séquence ANSI pour le texte en gras et en couleur noire sur fond noir
	formattedMessage = "\x1b[40m\x1b[1;37m" + formattedMessage + "\x1b[0m"

	return formattedMessage
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

	/* for _, client := range clients {
		if client != c {
			message := fmt.Sprintf("[%s] %s has joined the chat\n", getTimeStamp(), client.username)
			c.writer.WriteString(message)
			c.writer.Flush()
		}
	} */
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
