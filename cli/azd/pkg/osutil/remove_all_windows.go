// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

//go:build windows

package osutil

import (
	"context"
	"os"
)

// RemoveAll is like os.RemoveAll, but retries transient Windows file errors.
func RemoveAll(ctx context.Context, path string) error {
	return retryWindowsFileOperation(
		ctx,
		"remove of "+path,
		func() error {
			return os.RemoveAll(path)
		},
		defaultWindowsFileOperationBackoff(),
	)
}
