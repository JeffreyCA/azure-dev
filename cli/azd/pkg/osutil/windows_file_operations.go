// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

//go:build windows

package osutil

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/sethvargo/go-retry"
	"golang.org/x/sys/windows"
)

func retryWindowsFileOperation(
	ctx context.Context,
	description string,
	operation func() error,
	backoff retry.Backoff,
) error {
	return retry.Do(ctx, retry.WithMaxRetries(10, backoff), func(_ context.Context) error {
		err := operation()
		if errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
			errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			log.Printf("%s failed with a transient Windows file error, allowing retry: %v", description, err)
			return retry.RetryableError(err)
		}

		return err
	})
}

func defaultWindowsFileOperationBackoff() retry.Backoff {
	return retry.NewConstant(time.Second)
}
