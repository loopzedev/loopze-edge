// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).
// See LICENSE file for details.

// Package storage provides persistence for flows and encrypted credentials.
// The default implementation uses JSON files on the local filesystem,
// matching the single-binary, zero-dependency philosophy of LOOPZE.
package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Storage defines the persistence interface for flows and credentials.
// Implementations may store data in files, databases, or remote services.
type Storage interface {
	// LoadFlows reads all flow definitions from persistent storage.
	// Returns an empty slice (not nil) if no flows have been saved yet.
	LoadFlows() ([]flow.Flow, error)

	// SaveFlows writes the given flow definitions to persistent storage,
	// replacing any previously saved flows entirely.
	SaveFlows(flows []flow.Flow) error

	// LoadWorkspace reads the full workspace (flows + config nodes) from storage.
	// Handles backward compatibility: if the file contains the old []Flow format,
	// it is automatically wrapped into a Workspace.
	LoadWorkspace() (flow.Workspace, error)

	// SaveWorkspace writes the full workspace (flows + config nodes) to storage.
	// Always writes the new { "flows": [...], "configs": [...] } format.
	SaveWorkspace(ws flow.Workspace) error

	// LoadCredentials reads the raw (encrypted) credentials bytes from storage.
	// The caller is responsible for decryption via the credentials package.
	// Returns nil bytes and no error if no credentials file exists yet.
	LoadCredentials() ([]byte, error)

	// SaveCredentials writes the raw (encrypted) credentials bytes to storage.
	// The caller is responsible for encryption via the credentials package.
	SaveCredentials(data []byte) error

	// LoadUsers reads the raw user-records bytes from storage. The caller is
	// responsible for JSON unmarshalling. Returns nil bytes and no error if
	// no users file exists yet (this is the normal first-run state).
	LoadUsers() ([]byte, error)

	// SaveUsers writes the raw user-records bytes to storage. The caller is
	// responsible for marshalling to JSON. The bytes contain Argon2 password
	// hashes, so the file is written with 0600 permissions.
	SaveUsers(data []byte) error
}

// FileStorage implements the Storage interface using JSON files on disk.
// Flow definitions are stored as human-readable JSON. Credentials are stored
// as raw bytes (encrypted by the credentials.CredentialManager before being
// passed to SaveCredentials).
//
// All file operations are serialized with a mutex to prevent concurrent
// read/write corruption.
type FileStorage struct {
	flowFile        string
	credentialsFile string
	usersFile       string
	mu              sync.RWMutex
}

// NewFileStorage creates a FileStorage that reads and writes to the given paths.
// It ensures the parent directories exist for all files.
func NewFileStorage(flowFile, credentialsFile, usersFile string) (*FileStorage, error) {
	// Ensure parent directories exist for all files.
	for _, path := range []string{flowFile, credentialsFile, usersFile} {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("storage: failed to create directory %q: %w", dir, err)
		}
	}

	return &FileStorage{
		flowFile:        flowFile,
		credentialsFile: credentialsFile,
		usersFile:       usersFile,
	}, nil
}

// LoadFlows reads and parses the flow definitions JSON file.
// If the file does not exist, an empty slice is returned without error
// (this is the normal state on first startup).
func (fs *FileStorage) LoadFlows() ([]flow.Flow, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	data, err := os.ReadFile(fs.flowFile)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("no existing flow file found, starting with empty flows", "path", fs.flowFile)
			return []flow.Flow{}, nil
		}
		return nil, fmt.Errorf("storage: failed to read flow file %q: %w", fs.flowFile, err)
	}

	// Handle empty file gracefully.
	if len(data) == 0 {
		slog.Debug("flow file is empty, starting with empty flows", "path", fs.flowFile)
		return []flow.Flow{}, nil
	}

	var flows []flow.Flow
	if err := json.Unmarshal(data, &flows); err != nil {
		return nil, fmt.Errorf("storage: failed to parse flow file %q: %w", fs.flowFile, err)
	}

	slog.Info("flows loaded from storage", "path", fs.flowFile, "count", len(flows))
	return flows, nil
}

// SaveFlows serializes the given flows to JSON and writes them to the flow file.
// The file is written atomically by first writing to a temporary file in the
// same directory and then renaming it, to prevent corruption on crash.
func (fs *FileStorage) SaveFlows(flows []flow.Flow) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := json.MarshalIndent(flows, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: failed to marshal flows: %w", err)
	}

	if err := atomicWriteFile(fs.flowFile, data, 0644); err != nil {
		return fmt.Errorf("storage: failed to write flow file %q: %w", fs.flowFile, err)
	}

	slog.Info("flows saved to storage", "path", fs.flowFile, "count", len(flows))
	return nil
}

// LoadWorkspace reads and parses the workspace file, supporting both the new
// { "flows": [...], "configs": [...] } format and the legacy []Flow format.
// If the file starts with '[', it is parsed as []Flow and wrapped in a Workspace.
// If it starts with '{', it is parsed directly as a Workspace.
func (fs *FileStorage) LoadWorkspace() (flow.Workspace, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	data, err := os.ReadFile(fs.flowFile)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("no existing workspace file found, starting empty", "path", fs.flowFile)
			return flow.Workspace{Flows: []flow.Flow{}}, nil
		}
		return flow.Workspace{}, fmt.Errorf("storage: failed to read workspace file %q: %w", fs.flowFile, err)
	}

	if len(data) == 0 {
		slog.Debug("workspace file is empty, starting empty", "path", fs.flowFile)
		return flow.Workspace{Flows: []flow.Flow{}}, nil
	}

	// Detect format by first non-whitespace byte.
	trimmed := bytes.TrimLeft(data, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// Legacy format: flat []Flow array.
		var flows []flow.Flow
		if err := json.Unmarshal(data, &flows); err != nil {
			return flow.Workspace{}, fmt.Errorf("storage: failed to parse legacy flow file %q: %w", fs.flowFile, err)
		}
		slog.Info("workspace loaded (legacy format)", "path", fs.flowFile, "flows", len(flows))
		return flow.Workspace{Flows: flows}, nil
	}

	// New format: Workspace object.
	var ws flow.Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return flow.Workspace{}, fmt.Errorf("storage: failed to parse workspace file %q: %w", fs.flowFile, err)
	}
	if ws.Flows == nil {
		ws.Flows = []flow.Flow{}
	}

	slog.Info("workspace loaded", "path", fs.flowFile, "flows", len(ws.Flows), "configs", len(ws.Configs))
	return ws, nil
}

// SaveWorkspace serializes the workspace to the new { "flows", "configs" } format.
// Written atomically to prevent corruption.
func (fs *FileStorage) SaveWorkspace(ws flow.Workspace) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return fmt.Errorf("storage: failed to marshal workspace: %w", err)
	}

	if err := atomicWriteFile(fs.flowFile, data, 0644); err != nil {
		return fmt.Errorf("storage: failed to write workspace file %q: %w", fs.flowFile, err)
	}

	slog.Info("workspace saved", "path", fs.flowFile, "flows", len(ws.Flows), "configs", len(ws.Configs))
	return nil
}

// LoadCredentials reads the raw credentials file bytes. The returned data is
// expected to be encrypted — decryption is handled by the credentials package.
//
// Returns nil bytes and no error if the credentials file does not exist yet.
// This is the normal state on first startup before any credentials are saved.
//
// Encryption details (handled by credentials.CredentialManager):
//   - Algorithm: AES-256-GCM (authenticated encryption with associated data)
//   - Key source: separate key file (loopze.key), auto-generated on first run
//   - Format: nonce (12 bytes) || ciphertext || GCM authentication tag (16 bytes)
func (fs *FileStorage) LoadCredentials() ([]byte, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	data, err := os.ReadFile(fs.credentialsFile)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("no existing credentials file found", "path", fs.credentialsFile)
			return nil, nil
		}
		return nil, fmt.Errorf("storage: failed to read credentials file %q: %w", fs.credentialsFile, err)
	}

	slog.Debug("credentials loaded from storage", "path", fs.credentialsFile, "size", len(data))
	return data, nil
}

// SaveCredentials writes the raw (pre-encrypted) credentials bytes to the
// credentials file. The caller must encrypt the data before passing it here.
//
// The file is written with restrictive permissions (0600) since it contains
// encrypted secrets. Atomic write is used to prevent corruption.
func (fs *FileStorage) SaveCredentials(data []byte) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := atomicWriteFile(fs.credentialsFile, data, 0600); err != nil {
		return fmt.Errorf("storage: failed to write credentials file %q: %w", fs.credentialsFile, err)
	}

	slog.Debug("credentials saved to storage", "path", fs.credentialsFile, "size", len(data))
	return nil
}

// LoadUsers reads the raw user-records bytes from the users file. Returns
// nil bytes and no error if the file does not exist yet (first-run state).
// The caller is responsible for JSON unmarshalling.
func (fs *FileStorage) LoadUsers() ([]byte, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	data, err := os.ReadFile(fs.usersFile)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("no existing users file found", "path", fs.usersFile)
			return nil, nil
		}
		return nil, fmt.Errorf("storage: failed to read users file %q: %w", fs.usersFile, err)
	}

	slog.Debug("users loaded from storage", "path", fs.usersFile, "size", len(data))
	return data, nil
}

// SaveUsers writes the raw user-records bytes to the users file. The file
// contains Argon2id password hashes, so it is written with restrictive
// permissions (0600). Atomic write is used to prevent corruption.
func (fs *FileStorage) SaveUsers(data []byte) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := atomicWriteFile(fs.usersFile, data, 0600); err != nil {
		return fmt.Errorf("storage: failed to write users file %q: %w", fs.usersFile, err)
	}

	slog.Debug("users saved to storage", "path", fs.usersFile, "size", len(data))
	return nil
}

// atomicWriteFile writes data to a temporary file in the same directory as
// the target path and then renames it to the target. This ensures the file
// is either fully written or not modified at all, preventing partial writes
// if the process crashes mid-write.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".loopze-tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any error path.
	success := false
	defer func() {
		if !success {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("failed to set permissions on temp file: %w", err)
	}

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Ensure data is flushed to disk before rename.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to rename temp file to %q: %w", path, err)
	}

	success = true
	return nil
}
