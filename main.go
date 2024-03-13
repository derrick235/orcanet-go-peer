package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cbergoon/speedtest-go"
)

const keyServerAddr = "serverAddr"

func getRoot(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("got /root request\n")
	io.WriteString(w, "Hello, HTTP!\n")
}

func getFile(w http.ResponseWriter, r *http.Request) {
	// Get the context from the request
	ctx := r.Context()

	// Check if the "filename" query parameter is present
	hasFilename := r.URL.Query().Has("filename")

	// Retrieve the value of the "filename" query parameter
	filename := r.URL.Query().Get("filename")

	// Print information about the request
	fmt.Printf("%s: got /file request. filename(%t)=%s\n",
		ctx.Value(keyServerAddr),
		hasFilename, filename,
	)

	// Check if the "filename" parameter is present
	if hasFilename {
		// Check if the file exists in the local directory
		filePath := filepath.Join(".", filename)
		if _, err := os.Stat(filePath); err == nil {
			// Serve the file using http.ServeFile
			http.ServeFile(w, r, filePath)
			fmt.Printf("Served %s to client\n", filename)
			return
		} else if os.IsNotExist(err) {
			// File not found
			http.Error(w, "File not found", http.StatusNotFound)
			return
		} else {
			// Other error
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		// Write a response indicating that no filename was found
		io.WriteString(w, "No filename found\n")
	}
}

type FileData struct {
	FileName string `json:"filename"`
	Content  []byte `json:"content"`
}

func sendFile(w http.ResponseWriter, r *http.Request, confirming *bool, confirmation *string) {
	// Extract filename from URL path
	filename := r.URL.Path[len("/requestFile/"):]

	// Ask for confirmation
	*confirming = true
	fmt.Printf("\nYou have just received a request to send file '%s'. Do you want to send the file? (yes/no): ", filename)

	// Check if confirmation is received
	for *confirmation != "yes" {
		if *confirmation != "" {
			http.Error(w, fmt.Sprintf("Client declined to send file '%s'.", filename), http.StatusUnauthorized)
			*confirmation = ""
			*confirming = false
			return
		}
	}
	*confirmation = ""
	*confirming = false

	// Open the file
	file, err := os.Open("./files/stored/" + filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()

	// Set content type
	contentType := "application/octet-stream"
	switch {
	case filename[len(filename)-4:] == ".txt":
		contentType = "text/plain"
	case filename[len(filename)-5:] == ".json":
		contentType = "application/json"
	}

	// Set content disposition header
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Type", contentType)

	// Copy file contents to response body
	_, err = io.Copy(w, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("\nFile %s sent!\n\n> ", filename)
}

type PeerNodeData struct {
	isMe        bool
	balance     float64
	publicKey   string
	location    string
	isConnected bool
}

func getNodeInfo() {

}
func storeFile(w http.ResponseWriter, r *http.Request, confirming *bool, confirmation *string) {
	// Parse JSON object from request body
	var fileData FileData
	err := json.NewDecoder(r.Body).Decode(&fileData)
	if err != nil {
		http.Error(w, "Failed to parse JSON data", http.StatusBadRequest)
		return
	}

	// Ask for confirmation
	*confirming = true
	fmt.Printf("\nYou have just received a request to store file '%s'. Do you want to store the file? (yes/no): ", fileData.FileName)

	// Check if confirmation is received
	for *confirmation != "yes" {
		if *confirmation != "" {
			http.Error(w, fmt.Sprintf("Client declined to store file '%s'.", fileData.FileName), http.StatusUnauthorized)
			*confirmation = ""
			*confirming = false
			return
		}
	}
	*confirmation = ""
	*confirming = false

	// Create the directory if it doesn't exist
	err = os.MkdirAll("./files/stored/", 0755)
	if err != nil {
		return
	}

	// Create file
	file, err := os.Create("./files/stored/" + fileData.FileName)
	if err != nil {
		http.Error(w, "Failed to create file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Write content to file
	_, err = file.Write(fileData.Content)
	if err != nil {
		http.Error(w, "Failed to write to file", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "\nRequested client stored file %s successfully!\n", fileData.FileName)
	fmt.Printf("\nStored file %s!\n\n> ", fileData.FileName)
}

func getFileOnce(ip, port, filename string) {
	resp, err := http.Get(fmt.Sprintf("http://%s:%s/requestFile/%s", ip, port, filename))
	if err != nil {
		fmt.Printf("Error: %s\n\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response body:", err)
			return
		}
		fmt.Printf("\nError: %s\n> ", body)
		return
	}

	// Create the directory if it doesn't exist
	err = os.MkdirAll("./files/requested/", 0755)
	if err != nil {
		return
	}

	// Create file
	out, err := os.Create("./files/requested/" + filename)
	if err != nil {
		return
	}
	defer out.Close()

	// Write response body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return
	}

	fmt.Printf("\nFile %s downloaded successfully!\n\n> ", filename)
}

func requestStorage(ip, port, filename string) {
	// Read file content
	content, err := os.ReadFile("./files/documents/" + filename)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Create FileData struct
	fileData := FileData{
		FileName: filename,
		Content:  content,
	}

	// Marshal FileData to JSON
	jsonData, err := json.Marshal(fileData)
	if err != nil {
		fmt.Println("Error marshalling JSON:", err)
		return
	}

	// Send POST request to store file
	resp, err := http.Post(fmt.Sprintf("http://%s:%s/storeFile/", ip, port), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response body:", err)
			return
		}
		fmt.Printf("\nError: %s\n> ", body)
		return
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return
	}

	fmt.Println(string(body))
	fmt.Print("> ")
}

func importFile(filePath string) {
	// Extract filename from the provided file path
	_, fileName := filepath.Split(filePath)
	if fileName == "" {
		fmt.Print("\nProvided path is a directory, not a file\n\n> ")
		return
	}

	// Open the source file
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Print("\nFile does not exist\n\n> ")
		return
	}
	defer file.Close()

	// Create the directory if it doesn't exist
	err = os.MkdirAll("./files/stored/", 0755)
	if err != nil {
		return
	}

	// Save the file to the destination directory with the same filename
	destinationPath := filepath.Join("./files/stored/", fileName)
	destinationFile, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return
	}
	defer destinationFile.Close()

	// Copy the contents of the source file to the destination file
	_, err = io.Copy(destinationFile, file)
	if err != nil {
		return
	}

	fmt.Printf("\nFile '%s' imported successfully!\n\n> ", fileName)
}

// Ask user to enter a port and returns it
func getPort() string {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter a port number to start listening to requests: ")
	for {
		port, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			os.Exit(1)
		}
		port = strings.TrimSpace(port)

		// Validate port
		listener, err := net.Listen("tcp", ":"+port)
		if err == nil {
			defer listener.Close()
			return port
		}

		fmt.Print("Invalid port. Please enter a different port: ")
	}
}

// Start HTTP server
func startServer(port string, serverReady chan bool, confirming *bool, confirmation *string) {
	http.HandleFunc("/requestFile/", func(w http.ResponseWriter, r *http.Request) {
		sendFile(w, r, confirming, confirmation)
	})
	http.HandleFunc("/storeFile/", func(w http.ResponseWriter, r *http.Request) {
		storeFile(w, r, confirming, confirmation)
	})

	fmt.Printf("Listening on port %s...\n\n", port)
	serverReady <- true
	http.ListenAndServe(":"+port, nil)
}

// Start CLI
func startCLI(confirming *bool, confirmation *string) {
	fmt.Println("Dive In and Explore! Type 'help' for available commands.")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		text, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading from stdin:", err)
			continue
		}

		text = strings.TrimSpace(text)
		parts := strings.Fields(text)
		if len(parts) == 0 {
			continue
		}

		command := parts[0]
		args := parts[1:]

		if *confirming {
			switch command {
			case "yes":
				*confirmation = "yes"
			default:
				*confirmation = "no"
			}
			continue
		}

		switch command {
		case "get":
			if len(args) == 3 {
				go getFileOnce(args[0], args[1], args[2])
			} else {
				fmt.Println("Usage: get [ip] [port] [filename>]")
				fmt.Println()
			}
		case "store":
			if len(args) == 3 {
				go requestStorage(args[0], args[1], args[2])
			} else {
				fmt.Println("Usage: store [ip] [port] [filename]")
				fmt.Println()
			}
		case "import":
			if len(args) == 1 {
				go importFile(args[0])
			} else {
				fmt.Println("Usage: import [filepath]")
				fmt.Println()
			}
		case "location":
			fmt.Println(getLocationData())
		case "list":
			// TO-DO
		case "exit":
			fmt.Println("Exiting...")
			return
		case "help":
			fmt.Println("COMMANDS:")
			fmt.Println(" get [ip] [port] [filename]     Request a file")
			fmt.Println(" store [ip] [port] [filename]   Request storage of a file")
			fmt.Println(" import [filepath]              Import a file")
			fmt.Println(" list                           List all files you are storing")
			fmt.Println(" location						 Print your location")
			fmt.Println(" exit                           Exit the program")
			fmt.Println()
		default:
			fmt.Println("Unknown command. Type 'help' for available commands.")
			fmt.Println()
		}
	}
}

type NetworkStatus struct {
	downloadSpeedMbps float64
	uploadSpeedMbps   float64
	latencyMs         float64
}

func getNetworkInfo() NetworkStatus {
	user, _ := speedtest.FetchUserInfo()

	serverList, _ := speedtest.FetchServerList(user)
	targets, _ := serverList.FindServer([]int{})

	for _, s := range targets {
		s.PingTest()
		s.DownloadTest()
		s.UploadTest()
		return NetworkStatus{latencyMs: float64(s.Latency), downloadSpeedMbps: s.DLSpeed, uploadSpeedMbps: s.ULSpeed}
	}
	return NetworkStatus{}
}
func getLocationData() string {
	ipapiClient := http.Client{}

	req, err := http.NewRequest("GET", "https://ipapi.co/json/", nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("User-Agent", "ipapi.co/#go-v1.4.01")

	resp, err := ipapiClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	return string(body)
}

func main() {

	mux := http.NewServeMux()
	mux.HandleFunc("/", getRoot)
	mux.HandleFunc("/requestFile", getFile)

	ctx := context.Background()
	server := &http.Server{
		Addr:    ":3333",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("server closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server: %s\n", err)
		}
	}()
	fmt.Println("Welcome to Orcanet!")
	port := getPort()

	serverReady := make(chan bool)
	confirming := false
	confirmation := ""
	go startServer(port, serverReady, &confirming, &confirmation)
	<-serverReady

	startCLI(&confirming, &confirmation)
}
