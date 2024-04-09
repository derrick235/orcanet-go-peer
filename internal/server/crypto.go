package server

import (
	"os"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// LoadOrCreateKey loads an existing cryptographic key from a file, or generates a new one if the file does not exist.
// This function ensures that a peer can maintain a consistent identity across sessions by reusing the same key pair.
//
// Parameters:
// - path: The file system path where the key is stored or will be saved.
//
// Returns: The loaded or newly generated libp2pcrypto.PrivKey, and an error if the operation fails.
func LoadOrCreateKey(path string) (libp2pcrypto.PrivKey, error) {
	// Check if the key file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Key file does not exist, create a new key
		priv, _, err := libp2pcrypto.GenerateKeyPair(libp2pcrypto.RSA, 2048)
		if err != nil {
			return nil, err
		}

		// Save the newly generated key
		keyBytes, err := libp2pcrypto.MarshalPrivateKey(priv)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(path, keyBytes, 0600); err != nil {
			return nil, err
		}

		return priv, nil
	}

	// Key file exists, load the key
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	priv, err := libp2pcrypto.UnmarshalPrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	return priv, nil
}
