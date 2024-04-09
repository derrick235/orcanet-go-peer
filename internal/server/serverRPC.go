package server

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"orca-peer/internal/fileshare"
	pb "orcanet/market"
	"os"
	"time"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/multiformats/go-multiaddr"
)

var (
	clientMode = flag.Bool("client", false, "run this program in client mode")
	bootstrap  = flag.String("bootstrap", "", "multiaddresses to bootstrap to")
	addr       = flag.String("addr", "", "multiaddresses to listen to")
)

type DHTvalue struct {
	Ids    string
	Names  string
	Ips    string
	Ports  string
	Prices string
}

type CustomValidator struct{}

func (cv CustomValidator) Select(string, [][]byte) (int, error) {
	return 0, nil
}

func (cv CustomValidator) Validate(key string, value []byte) error {
	//  length := 10
	//  hexRegex := regexp.MustCompile("^[0-9a-fA-F]{" + strconv.Itoa(length) + "}$")
	//  if !hexRegex.MatchString(key) {
	// 	 return errors.New("input is not a valid hexadecimal string or does not match the expected length")
	//  }
	return nil
}

func CreateDHTConnection(bootstrapAddress *string) (context.Context, *dht.IpfsDHT) {
	ctx := context.Background()
	flag.Parse()

	// Construct listening address
	var listenAddrString string
	if *addr == "" {
		listenAddrString = "/ip4/0.0.0.0/tcp/0"
	} else {
		listenAddrString = *addr
	}
	listenAddr, _ := multiaddr.NewMultiaddr(listenAddrString)

	var bootstrapPeers []multiaddr.Multiaddr

	// Connect to bootstrap peers
	if len(*bootstrap) > 0 {
		bootstrapAddr, err := multiaddr.NewMultiaddr(*bootstrap)
		if err != nil {
			fmt.Errorf("Invalid bootstrap address: %s", err)
		}
		bootstrapPeers = append(bootstrapPeers, bootstrapAddr)
	}

	// Create host
	privKey, err := LoadOrCreateKey("./peer_identity.key")
	if err != nil {
		panic(err)
	}

	host, err := libp2p.New(libp2p.ListenAddrs(listenAddr), libp2p.Identity(privKey))
	if err != nil {
		fmt.Errorf("Failed to create host: %s", err)
	}

	for _, addr := range host.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, host.ID())
		fmt.Println("Listen address:", fullAddr)
	}
	// Initialize the DHT
	var kademliaDHT *dht.IpfsDHT
	if *clientMode {
		kademliaDHT, err = dht.New(ctx, host, dht.Mode(dht.ModeClient))
	} else {
		kademliaDHT, err = dht.New(ctx, host, dht.Mode(dht.ModeServer))
	}
	if err != nil {
		fmt.Errorf("Failed to create DHT: %s", err)
		return nil, nil
	}

	// Connect the validator
	kademliaDHT.Validator = &CustomValidator{}

	// Bootstrap the DHT
	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		fmt.Errorf("Failed to bootstrap DHT: %s", err)
	}

	connectToBootstrapPeers(ctx, host, bootstrapPeers)

	// Create the user, connect to peers and run CLI
	user := promptForUserInfo(ctx)
	routingDiscovery := drouting.NewRoutingDiscovery(kademliaDHT)

	fmt.Println("Looking for existence of peers on the network before proceeding...")
	checkPeerExistence(ctx, host, kademliaDHT, routingDiscovery)
	fmt.Println("Peer(s) found! proceeding with the application.")

	return ctx, kademliaDHT
}

// promptForUserInfo creates a user struct based on information
// entered by the user in the terminal
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
//
// Returns: A *pb.User containing the ID, name, IP address, port, and price of the user
func promptForUserInfo(ctx context.Context) *pb.User {
	var username string
	fmt.Print("Enter username: ")
	fmt.Scanln(&username)

	// Generate a random ID for new user
	userID := fmt.Sprintf("user%d", rand.Intn(10000))

	fmt.Print("Enter a price for supplying files: ")
	var price int64
	fmt.Scanln(&price)

	user := &pb.User{
		Id:    userID,
		Name:  username,
		Ip:    "localhost",
		Port:  416320,
		Price: price,
	}

	return user
}

func sendFileToConsumer(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		for k, v := range r.URL.Query() {
			fmt.Printf("%s: %s\n", k, v)
		}
		// file = r.URL.Query().Get("filename")
		w.Write([]byte("Received a GET request\n"))

	default:
		w.WriteHeader(http.StatusNotImplemented)
		w.Write([]byte(http.StatusText(http.StatusNotImplemented)))
	}
	w.Write([]byte("Received a GET request\n"))
	filename := r.URL.Path[len("/reqFile/"):]

	// Open the file
	file, err := os.Open(filename)
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
		// Add more cases for other file types if needed
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
}

func runNotifyStore(client fileshare.FileShareClient, file *fileshare.FileDesc) *fileshare.StorageACKResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ackResponse, err := client.NotifyFileStore(ctx, file)
	if err != nil {
		log.Fatalf("client.NotifyFileStorage failed: %v", err)
	}
	log.Printf("ACK Response: %v", ackResponse)
	return ackResponse
}

func runNotifyUnstore(client fileshare.FileShareClient, file *fileshare.FileDesc) *fileshare.StorageACKResponse {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ackResponse, err := client.NotifyFileUnstore(ctx, file)
	if err != nil {
		log.Fatalf("client.NotifyFileStorage failed: %v", err)
	}
	log.Printf("ACK Response: %v", ackResponse)
	return ackResponse
}

func NotifyStoreWrapper(client fileshare.FileShareClient, file_name_hash string, file_name string, file_size_bytes int64, file_origin_address string, origin_user_id string, file_cost float32, file_data_hash string, file_bytes []byte) {
	var file_description = fileshare.FileDesc{FileNameHash: file_name_hash,
		FileName:          file_name,
		FileSizeBytes:     file_size_bytes,
		FileOriginAddress: file_origin_address,
		OriginUserId:      origin_user_id,
		FileCost:          file_cost,
		FileDataHash:      file_data_hash,
		FileBytes:         file_bytes}
	var ack = runNotifyUnstore(client, &file_description)
	if ack.IsAcknowledged {
		fmt.Printf("[Server]: Market acknowledged stopping storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	} else {
		fmt.Printf("[Server]: Unable to notify market that we are stopping the storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	}
}
func NotifyUnstoreWrapper(client fileshare.FileShareClient, file_name_hash string, file_name string, file_size_bytes int64, file_origin_address string, origin_user_id string, file_cost float32, file_data_hash string, file_bytes []byte) {
	var file_description = fileshare.FileDesc{FileNameHash: file_name_hash,
		FileName:          file_name,
		FileSizeBytes:     file_size_bytes,
		FileOriginAddress: file_origin_address,
		OriginUserId:      origin_user_id,
		FileCost:          file_cost,
		FileDataHash:      file_data_hash,
		FileBytes:         file_bytes}
	var ack = runNotifyUnstore(client, &file_description)
	if ack.IsAcknowledged {
		fmt.Printf("[Server]: Market acknowledged stopping storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	} else {
		fmt.Printf("[Server]: Unable to notify market that we are stopping the storage of file %s with hash %s \n", ack.FileName, ack.FileHash)
	}
}
