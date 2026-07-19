// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

//go:build !windows

package osutil

import (
	"context"
	"os"
)

// RemoveAll is like os.RemoveAll. The context is ignored on non-Windows platforms.
func RemoveAll(ctx context.Context, path string) error {
	return os.RemoveAll(path)
}
