// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

//go:build windows

package osutil

import (
	"context"
	"os"
)

// Rename is like os.Rename except it will retry the operation, up to 10 times, waiting a second between each retry when the
// Rename fails due to what may be transient file system errors. This can help work around issues where the file may
// temporary be opened by a virus scanner or some other process which prevents us from renaming the file.
func Rename(ctx context.Context, old, new string) error {
	return retryWindowsFileOperation(
		ctx,
		"rename of "+old+" to "+new,
		func() error {
			return os.Rename(old, new)
		},
		defaultWindowsFileOperationBackoff(),
	)
}
