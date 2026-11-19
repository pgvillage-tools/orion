// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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

package keeper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/store"
)

// Config is the configuration of a keeper
type Config struct {
	store.CommonConfig

	UID                   string
	DataDir               string
	WALDir                string
	PGListenAddress       string
	PGAdvertiseAddress    string
	PGPort                string
	PGAdvertisePort       string
	PGBinPath             string
	PGReplConnType        string
	PGReplAuthMethod      string
	PGReplLocalAuthMethod string
	PGReplSSLMode         string
	PGReplUsername        string
	PGReplPassword        string
	PGReplPasswordFile    string
	PGSUConnType          string
	PGSUAuthMethod        string
	PGSULocalAuthMethod   string
	PGSUUsername          string
	PGSUPassword          string
	PGSUPasswordFile      string

	CanBeMaster             bool
	CanBeSynchronousReplica bool
	DisableDataDirLocking   bool
}

// validAuthMethods are the auth methods the keeper accepts for the superuser and the replication user
var validAuthMethods = map[string]struct{}{authTrust: {}, authMd5: {}, authCert: {}, authIdent: {}, authPeer: {}}

// validConnectionTypes are the pg_hba connection types the keeper accepts for the superuser and the replication user
var validConnectionTypes = map[string]struct{}{
	connTypeHost: {}, connTypeHostSsl: {}, connTypeHostNoSsl: {}, connTypeHostGssEnc: {}, connTypeHostNoGssEnc: {},
}

// ValidateUsersConfig checks the superuser and replication user settings of c: connection types, auth methods and
// passwords. It also fills in the defaults of the local auth methods, reads the password files, and removes trailing
// new lines from the passwords.
func ValidateUsersConfig(ctx context.Context, c *Config) error {
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	var err error
	if c.PGSULocalAuthMethod == "" {
		c.PGSULocalAuthMethod = c.PGSUAuthMethod
	}
	if c.PGReplLocalAuthMethod == "" {
		c.PGReplLocalAuthMethod = c.PGReplAuthMethod
	}
	if _, ok := validConnectionTypes[c.PGReplConnType]; !ok {
		return errors.New("--pg-repl-connection-type must be one of: " +
			"host, hostssl, hostnossl, hostgssenc, hostnogssenc")
	}
	if _, ok := validConnectionTypes[c.PGSUConnType]; !ok {
		return errors.New("--pg-su-connection-type must be one of: host, hostssl, hostnossl, hostgssenc, hostnogssenc")
	}
	if _, ok := validAuthMethods[c.PGReplAuthMethod]; !ok {
		return errors.New("--pg-repl-auth-method must be one of: md5, trust, cert, ident or peer")
	}
	if c.PGReplUsername == "" {
		return errors.New("--pg-repl-username is required")
	}
	if _, ok := validAuthMethods[c.PGReplLocalAuthMethod]; !ok {
		return errors.New("--pg-repl-local-auth-method must be one of: md5, trust, cert, ident or peer")
	}
	if c.PGReplAuthMethod == authTrust {
		logger.Warn().
			Msg("not utilizing a password for replication between hosts is extremely dangerous")
		if c.PGReplPassword != "" || c.PGReplPasswordFile != "" {
			// revive:disable-next-line
			return errors.New("can not utilize --pg-repl-auth-method trust together with --pg-repl-password " +
				"or --pg-repl-passwordfile")
		}
	}
	if c.PGSUAuthMethod == authTrust || c.PGSULocalAuthMethod == authTrust {
		logger.Warn().Msg("not utilizing a password for superuser is extremely dangerous")
		if (c.PGSUAuthMethod == authTrust ||
			c.PGSULocalAuthMethod == authTrust) && (c.PGSUPassword != "" ||
			c.PGSUPasswordFile != "") {
			// revive:disable-next-line
			return errors.New("can not utilize --pg-su-auth-method trust and --pg-su-auth-method trust together with --pg-su-password or --pg-su-passwordfile")
		}
	}
	if c.PGReplAuthMethod == authMd5 && c.PGReplPassword == "" && c.PGReplPasswordFile == "" {
		return errors.New("one of --pg-repl-password or --pg-repl-passwordfile is required")
	}
	if c.PGReplAuthMethod == authMd5 && c.PGReplPassword != "" && c.PGReplPasswordFile != "" {
		return errors.New("only one of --pg-repl-password or --pg-repl-passwordfile must be provided")
	}
	if _, ok := validAuthMethods[c.PGSUAuthMethod]; !ok {
		return errors.New("--pg-su-auth-method must be one of: md5, trust, cert, ident or peer")
	}
	if c.PGSUAuthMethod == authMd5 && c.PGSUPassword == "" && c.PGSUPasswordFile == "" {
		return errors.New("one of --pg-su-password or --pg-su-passwordfile is required")
	}
	if c.PGSUAuthMethod == authMd5 && c.PGSUPassword != "" && c.PGSUPasswordFile != "" {
		return errors.New("only one of --pg-su-password or --pg-su-passwordfile must be provided")
	}
	if _, ok := validAuthMethods[c.PGSULocalAuthMethod]; !ok {
		return errors.New("--pg-su-local-auth-method must be one of: md5, trust, cert, ident or peer")
	}

	if c.PGReplPasswordFile != "" {
		c.PGReplPassword, err = readPasswordFromFile(ctx, c.PGReplPasswordFile)
		if err != nil {
			return fmt.Errorf("cannot read pg replication user password: %w", err)
		}
	}
	if c.PGSUPasswordFile != "" {
		c.PGSUPassword, err = readPasswordFromFile(ctx, c.PGSUPasswordFile)
		if err != nil {
			return fmt.Errorf("cannot read pg superuser password: %w", err)
		}
	}

	// Trim trailing new lines from passwords
	tp := strings.TrimRight(c.PGSUPassword, "\r\n")
	if c.PGSUPassword != tp {
		logger.Warn().Msg("superuser password contain trailing new line, removing")
		if tp == "" {
			return errors.New("superuser password is empty after removing trailing new line")
		}
		c.PGSUPassword = tp
	}

	tp = strings.TrimRight(c.PGReplPassword, "\r\n")
	if c.PGReplPassword != tp {
		logger.Warn().Msg("replication user password contain trailing new line, removing")
		if tp == "" {
			return errors.New("replication user password is empty after removing trailing new line")
		}
		c.PGReplPassword = tp
	}

	if c.PGSUUsername == c.PGReplUsername {
		logger.Warn().
			Msg("superuser name and replication user name are the same. Different users are suggested.")
		if c.PGReplAuthMethod != c.PGSUAuthMethod {
			return errors.New("do not support different auth methods when utilizing superuser for replication.")
		}
		if c.PGSUPassword != c.PGReplPassword && c.PGSUAuthMethod == authMd5 && c.PGReplAuthMethod == authMd5 {
			// revive:disable-next-line
			return errors.New("provided superuser name and replication user name are the same but provided passwords are different")
		}
	}
	return nil
}

// DefaultConnType is the default pg_hba connection type of the superuser and the replication user
const DefaultConnType = connTypeHost

// DefaultAuthMethod is the default auth method of the superuser and the replication user
const DefaultAuthMethod = authMd5

func readPasswordFromFile(ctx context.Context, filePath string) (string, error) {
	// Stat, not Lstat: a symlink (as Kubernetes mounts a Secret) has the mode of the file it points to
	fi, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filePath, err)
	}
	_, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)

	if fi.Mode().Perm()&groupOtherPermissions != 0 {
		// TODO: enforce this by exiting with an error
		logger.Warn().Str("file", filePath).Str("mode", fmt.Sprintf("%#o", fi.Mode().Perm())).
			Msgf("password file permissions are too open. " +
				"This file should only be readable to the user executing orion! Continuing...")
	}

	pwBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read password from file %s: %v", filePath, err)
	}
	return string(pwBytes), nil
}
