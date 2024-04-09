package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	pb "orcanet/market"

	dht "github.com/libp2p/go-libp2p-kad-dht"
)

// printRoutingTable prints the current state of the DHT's routing table to the console.
// This function is useful for debugging and monitoring the local view of the network topology.
//
// Parameters:
// - dht: A pointer to the dht.IpfsDHT instance whose routing table is to be printed.
//
// Returns: None.
func printRoutingTable(dht *dht.IpfsDHT) {
	for _, peer := range dht.RoutingTable().ListPeers() {
		fmt.Println("Peer ID:", peer)
	}
}

// registerFile registers a file in the DHT, indicating that the local user holds a specific file.
// This operation makes the file discoverable to other peers searching for it through the DHT.
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
// - dht: A pointer to the dht.IpfsDHT used for the registration.
// - req: A *pb.RegisterFileRequest containing the user's information and the file hash to register.
//
// Returns: An error if the registration fails, or nil on success.
func registerFile(ctx context.Context, dht *dht.IpfsDHT, req *pb.RegisterFileRequest) error {
	//get existing data from DHT
	key := fmt.Sprintf("/market/file/%s", req.FileHash)
	dataChan, _ := dht.SearchValue(ctx, key)

	var rawData []byte
	for data := range dataChan {
		rawData = data // Assuming data is a byte slice containing JSON data
		break          //we only expect one result
	}

	var result DHTvalue
	_ = json.Unmarshal(rawData, &result)

	ids := strings.Split(result.Ids, "|")

	// check if the user already placed this file in
	for _, id := range ids {
		if id == req.User.Id {
			errMsg := "You have already placed this file in the DHT."
			fmt.Println(errMsg)
			return fmt.Errorf(errMsg)
		}
	}

	var appendedResult DHTvalue = DHTvalue{
		Ids:    result.Ids + req.User.Id + "|",
		Names:  result.Names + req.User.Name + "|",
		Ips:    result.Ips + req.User.Ip + "|",
		Ports:  result.Ports + strconv.Itoa(int(req.User.Port)) + "|",
		Prices: result.Prices + strconv.Itoa(int(req.User.Price)) + "|",
	}

	//serialize the User object to byte slice for storage
	data, err := json.Marshal(appendedResult)

	if err != nil {
		errMsg := fmt.Sprintf("Error marshaling user data for file hash %s: %v", req.FileHash, err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	if err := dht.PutValue(ctx, key, data); err != nil {
		errMsg := fmt.Sprintf("Error putting value in the DHT for file hash %s: %v", req.FileHash, err)
		fmt.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	fmt.Printf("Successfully registered file with hash %s\n", req.FileHash)
	return nil
}

// checkHolders retrieves a list of users holding a specific file by querying the DHT.
// This function is part of the file discovery process, allowing peers to locate others
// that have the file they are looking for.
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
// - dht: A pointer to the dht.IpfsDHT used for the query.
// - req: A *pb.CheckHoldersRequest containing the file hash to search for.
//
// Returns: A *pb.HoldersResponse containing the list of Users holding the file, and an error if the query fails.
func checkHolders(ctx context.Context, dht *dht.IpfsDHT, req *pb.CheckHoldersRequest) (*pb.HoldersResponse, error) {
	key := fmt.Sprintf("/market/file/%s", req.FileHash)

	dataChan, err := dht.SearchValue(ctx, key)

	if err != nil {
		errMsg := fmt.Sprintf("Error searching for file %s", req.FileHash)
		fmt.Println(errMsg)
		return nil, fmt.Errorf(errMsg)
	}

	var rawData []byte
	for data := range dataChan {
		rawData = data // Assuming data is a byte slice containing JSON data
		break          //we only expect one result
	}

	var result DHTvalue
	_ = json.Unmarshal(rawData, &result)

	ids := strings.Split(result.Ids, "|")
	names := strings.Split(result.Names, "|")
	ips := strings.Split(result.Ips, "|")
	ports := strings.Split(result.Ports, "|")
	prices := strings.Split(result.Prices, "|")

	var holders = []*pb.User{}

	if !(len(ids) == len(names) && len(names) == len(ips) && len(ips) == len(ports) && len(ports) == len(prices)) {
		fmt.Println("Issue with dht returns - unequal data")
		return &pb.HoldersResponse{Holders: holders}, nil
	}

	if len(ids) <= 1 { //one because split will have an extra empty element if the key exists
		return &pb.HoldersResponse{Holders: holders}, nil
	}

	for i := 0; i < len(ids)-1; i++ {
		port, _ := strconv.Atoi(ports[i])
		price, _ := strconv.Atoi(prices[i])

		user := &pb.User{
			Id:    ids[i],
			Name:  names[i],
			Ip:    ips[i],
			Port:  int32(port),
			Price: int64(price),
		}

		holders = append(holders, user)
	}

	return &pb.HoldersResponse{Holders: holders}, nil
}
