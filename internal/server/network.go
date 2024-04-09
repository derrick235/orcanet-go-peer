package server

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	"github.com/multiformats/go-multiaddr"
)

// connectToBootstrapPeers connects the local host to a set of bootstrap peers.
// This function is intended to help the local peer join the network by establishing
// connections with known, reliable nodes. The bootstrap peers are specified
// by the -bootstrap flag and provided to the function via the bootstrapPeers parameter.
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
// - host: The local host.Host instance representing the peer that's connecting to the bootstrap nodes.
// - bootstrapPeers: A slice of multiaddr.Multiaddr representing the network addresses of the bootstrap peers.
//
// Returns: None.
func connectToBootstrapPeers(ctx context.Context, host host.Host, bootstrapPeers []multiaddr.Multiaddr) {
	var wg sync.WaitGroup
	for _, peerAddr := range bootstrapPeers {
		peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := host.Connect(ctx, *peerinfo); err != nil {
				fmt.Errorf("Failed to connect to bootstrap peer %v: %s", peerinfo, err)
			} else {
				fmt.Printf("Connected to bootstrap peer: %s\n", peerinfo.ID)
			}
		}()
	}
	wg.Wait()
}

// checkPeerExistence checks if at least one peer can be discovered in the network
// using the provided host and discovery mechanism. This function can be used to
// verify network connectivity and the ability to find peers.
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
// - host: The host.Host instance used for peer discovery.
// - dht: A pointer to the dht.IpfsDHT instance used for direct DHT operations.
// - routingDiscovery: A pointer to the drouting.RoutingDiscovery used for peer discovery.
//
// Returns: A bool indicating whether at least one peer has been discovered.
func checkPeerExistence(ctx context.Context, host host.Host, dht *dht.IpfsDHT, routingDiscovery *drouting.RoutingDiscovery) bool {
	if len(dht.RoutingTable().ListPeers()) > 0 {
		return true
	}

	for {
		isPeersFound := peerDiscovery(ctx, host, dht, routingDiscovery)
		if isPeersFound {
			return true
		}
		fmt.Println("No peers found, waiting...")
		time.Sleep(7 * time.Second)
	}
}

// peerDiscovery attempts to discover new peers in the network using the provided host and discovery mechanism.
// It actively searches for peers advertising themselves under a specific identifier and tries to connect to them.
// This is part of the network's dynamic peer discovery process.
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
// - host: The host.Host instance used for peer discovery.
// - dht: A pointer to the dht.IpfsDHT instance used for direct DHT operations.
// - routingDiscovery: A pointer to the drouting.RoutingDiscovery used for peer discovery.
//
// Returns: A bool indicating whether at least one new peer has been successfully discovered and connected.
func peerDiscovery(ctx context.Context, host host.Host, dht *dht.IpfsDHT, routingDiscovery *drouting.RoutingDiscovery) bool {
	dutil.Advertise(ctx, routingDiscovery, "market")

	peerChan, err := routingDiscovery.FindPeers(ctx, "market")
	if err != nil {
		fmt.Println("Failed to find peers:", err)
		return false
	}

	peerDiscovered := false
	for peer := range peerChan {
		if peer.ID == host.ID() {
			continue
		}

		success := tryConnectWithBackoff(ctx, host, peer, 3, 1*time.Second)
		if success {
			peerDiscovered = true
		} else {
			dht.RoutingTable().RemovePeer(peer.ID)
			fmt.Printf("Peer %s has left the network \n", peer.ID)
		}
	}
	return peerDiscovered
}

// tryConnectWithBackoff attempts to connect to a specified peer with exponential backoff.
// This function makes repeated attempts to connect, doubling the wait time after each failure,
// up to a maximum number of retries. This strategy helps to mitigate temporary network issues
// or peer unavailability.
//
// Parameters:
// - ctx: A context.Context for controlling the function's execution lifetime.
// - host: The local host.Host instance attempting the connection.
// - peer: The peer.AddrInfo representing the target peer for the connection.
// - maxRetries: The maximum number of connection attempts to make.
// - initialBackoff: The initial waiting period before retrying after a failed connection attempt.
//
// Returns: A bool indicating whether the connection attempt was successful.
func tryConnectWithBackoff(ctx context.Context, host host.Host, peer peer.AddrInfo, maxRetries int, initialBackoff time.Duration) bool {
	backoff := initialBackoff
	for i := 0; i < maxRetries; i++ {
		err := host.Connect(ctx, peer)
		if err == nil {
			return true
		}
		fmt.Printf("Attempt %d: Failed connecting to %s, error: %s\n", i+1, peer.ID, err)

		time.Sleep(backoff + time.Duration(rand.Intn(1000))*time.Millisecond)
		backoff *= 2
	}
	return false
}
