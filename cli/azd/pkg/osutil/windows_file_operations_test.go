// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

//go:build windows

package osutil

import (
	"errors"
	"testing"

	"github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestRetryWindowsFileOperation(t *testing.T) {
	attempts := 0

	err := retryWindowsFileOperation(
		t.Context(),
		"test operation",
		func() error {
			attempts++
			if attempts < 3 {
				return windows.ERROR_ACCESS_DENIED
			}
			return nil
		},
		retry.NewConstant(0),
	)

	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestRetryWindowsFileOperationDoesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	expectedErr := errors.New("permanent failure")

	err := retryWindowsFileOperation(
		t.Context(),
		"test operation",
		func() error {
			attempts++
			return expectedErr
		},
		retry.NewConstant(0),
	)

	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, 1, attempts)
}
