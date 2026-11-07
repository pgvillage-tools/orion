// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package tls

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	privateDirMode  os.FileMode = 0o700
	privateFileMode os.FileMode = 0o600
	publicFileMode  os.FileMode = 0o644
)

// writeFile writes data to a temporary file next to path, and renames it to path. The directory is created (only
// accessible by the owner) when it doesn't exist.
func writeFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, privateDirMode); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer func() {
		// Only fails when the rename succeeded
		_ = os.Remove(tmp.Name())
	}()
	if err = tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// WriteFile writes data to path (atomically, with a temporary file). Use private for private keys: they are only
// readable by the owner.
func WriteFile(path string, data []byte, private bool) error {
	mode := publicFileMode
	if private {
		mode = privateFileMode
	}
	return writeFile(path, data, mode)
}
