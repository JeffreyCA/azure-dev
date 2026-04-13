// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package exterrors

import (
	"context"
	"errors"
	"testing"

	"github.com/azure/azure-dev/cli/azd/pkg/azdext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFromAiService(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		fallbackCode string
		wantCategory azdext.LocalErrorCategory
		wantCode     string
		wantSuggest  string
	}{
		{
			name:         "Unauthenticated returns Auth with not_logged_in",
			err:          status.Error(codes.Unauthenticated, "not logged in, run `azd auth login` to login"),
			fallbackCode: "model_catalog_failed",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeNotLoggedIn,
		},
		{
			name:         "Unauthenticated returns Auth with login_expired",
			err:          status.Error(codes.Unauthenticated, "AADSTS70043: token expired\nlogin expired, run `azd auth login`"),
			fallbackCode: "model_catalog_failed",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeLoginExpired,
		},
		{
			name:         "Unauthenticated token protection returns token protection guidance",
			err:          status.Error(codes.Unauthenticated, "AADSTS530084: Access has been blocked by conditional access token protection policy configured by this organization. See https://aka.ms/TBCADocs"),
			fallbackCode: "model_catalog_failed",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeTokenProtectionBlocked,
			wantSuggest:  "won't resolve this",
		},
		{
			name:         "Unauthenticated returns Auth with generic auth_failed",
			err:          status.Error(codes.Unauthenticated, "insufficient permissions for this operation"),
			fallbackCode: "model_catalog_failed",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeAuthFailed,
		},
		{
			name:         "Other gRPC error returns Internal",
			err:          status.Error(codes.InvalidArgument, "missing subscription"),
			fallbackCode: "model_catalog_failed",
			wantCategory: azdext.LocalErrorCategoryInternal,
			wantCode:     "model_catalog_failed",
		},
		{
			name:         "Canceled returns User cancellation",
			err:          status.Error(codes.Canceled, "cancelled"),
			fallbackCode: "model_catalog_failed",
			wantCategory: azdext.LocalErrorCategoryUser,
			wantCode:     CodeCancelled,
		},
		{
			name:         "Nil returns nil",
			err:          nil,
			fallbackCode: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromAiService(tt.err, tt.fallbackCode)
			if tt.err == nil {
				assert.Nil(t, result)
				return
			}

			var localErr *azdext.LocalError
			require.ErrorAs(t, result, &localErr)
			assert.Equal(t, tt.wantCategory, localErr.Category)
			assert.Equal(t, tt.wantCode, localErr.Code)
			if tt.wantSuggest != "" {
				assert.Contains(t, localErr.Suggestion, tt.wantSuggest)
			}
		})
	}
}

func TestFromPrompt(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		contextMsg   string
		wantCategory azdext.LocalErrorCategory
		wantCode     string
		wantContain  string
		wantSuggest  string
	}{
		{
			name:         "Auth error returns structured Auth error with context",
			err:          status.Error(codes.Unauthenticated, "not logged in, run `azd auth login` to login"),
			contextMsg:   "failed to prompt for subscription",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeNotLoggedIn,
			wantContain:  "failed to prompt for subscription",
		},
		{
			name:         "Login expired returns structured Auth error with context",
			err:          status.Error(codes.Unauthenticated, "AADSTS70043: token expired"),
			contextMsg:   "failed to prompt for location",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeLoginExpired,
			wantContain:  "failed to prompt for location",
		},
		{
			name:         "Token protection returns structured Auth error with context",
			err:          status.Error(codes.Unauthenticated, "AADSTS530084: Access has been blocked by conditional access token protection policy configured by this organization. See https://aka.ms/TBCADocs"),
			contextMsg:   "failed to prompt for subscription",
			wantCategory: azdext.LocalErrorCategoryAuth,
			wantCode:     CodeTokenProtectionBlocked,
			wantContain:  "failed to prompt for subscription",
			wantSuggest:  "won't resolve this",
		},
		{
			name:         "Cancellation returns User error",
			err:          context.Canceled,
			contextMsg:   "subscription selection was cancelled",
			wantCategory: azdext.LocalErrorCategoryUser,
			wantCode:     CodeCancelled,
		},
		{
			name:        "Non-auth error returns wrapped error",
			err:         status.Error(codes.Internal, "server error"),
			contextMsg:  "failed to prompt for subscription",
			wantContain: "failed to prompt for subscription",
		},
		{
			name: "Nil returns nil",
			err:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FromPrompt(tt.err, tt.contextMsg)
			if tt.err == nil {
				assert.Nil(t, result)
				return
			}

			if tt.wantCategory != "" {
				var localErr *azdext.LocalError
				require.ErrorAs(t, result, &localErr)
				assert.Equal(t, tt.wantCategory, localErr.Category)
				assert.Equal(t, tt.wantCode, localErr.Code)
				if tt.wantSuggest != "" {
					assert.Contains(t, localErr.Suggestion, tt.wantSuggest)
				}
			}
			if tt.wantContain != "" {
				assert.Contains(t, result.Error(), tt.wantContain)
			}
		})
	}
}

func TestWrapTokenProtectionError(t *testing.T) {
	t.Run("matching error returns structured auth error", func(t *testing.T) {
		result := WrapTokenProtectionError(
			errors.New("AADSTS530084: blocked by conditional access token protection policy. See https://aka.ms/TBCADocs"),
			"failed to retrieve user profile",
		)

		var localErr *azdext.LocalError
		require.ErrorAs(t, result, &localErr)
		assert.Equal(t, azdext.LocalErrorCategoryAuth, localErr.Category)
		assert.Equal(t, CodeTokenProtectionBlocked, localErr.Code)
		assert.Contains(t, localErr.Message, "failed to retrieve user profile")
		assert.Contains(t, localErr.Suggestion, tokenProtectionDocsLink)
	})

	t.Run("docs link only matches without AADSTS code", func(t *testing.T) {
		result := WrapTokenProtectionError(
			errors.New("token request blocked. See https://aka.ms/TBCADocs for details"),
			"graph call failed",
		)

		var localErr *azdext.LocalError
		require.ErrorAs(t, result, &localErr)
		assert.Equal(t, CodeTokenProtectionBlocked, localErr.Code)
		assert.Contains(t, localErr.Message, "graph call failed")
	})

	t.Run("user-friendly message matches via token protection policy phrase", func(t *testing.T) {
		// Simulates the case where azidentity extracts the rendered message from azd stderr,
		// which may not contain the raw AADSTS code or docs link.
		result := WrapTokenProtectionError(
			errors.New("AzureDeveloperCLICredential: A Conditional Access token protection policy blocked this Microsoft Graph token request."),
			"failed to discover agent identity",
		)

		var localErr *azdext.LocalError
		require.ErrorAs(t, result, &localErr)
		assert.Equal(t, CodeTokenProtectionBlocked, localErr.Code)
		assert.Contains(t, localErr.Message, "failed to discover agent identity")
	})

	t.Run("non-matching error returns nil", func(t *testing.T) {
		assert.Nil(t, WrapTokenProtectionError(errors.New("plain network timeout"), "ignored"))
	})

	t.Run("nil error returns nil", func(t *testing.T) {
		assert.Nil(t, WrapTokenProtectionError(nil, "ignored"))
	})
}

func TestGraphError(t *testing.T) {
	t.Run("token protection error returns structured auth error", func(t *testing.T) {
		result := GraphError(
			errors.New("AADSTS530084: blocked by token protection policy"),
			"failed to discover agent identity",
		)

		var localErr *azdext.LocalError
		require.ErrorAs(t, result, &localErr)
		assert.Equal(t, CodeTokenProtectionBlocked, localErr.Code)
		assert.Contains(t, localErr.Message, "failed to discover agent identity")
	})

	t.Run("non-matching error returns wrapped error", func(t *testing.T) {
		result := GraphError(
			errors.New("network timeout"),
			"failed to discover agent identity",
		)

		require.Error(t, result)
		assert.Contains(t, result.Error(), "failed to discover agent identity")
		assert.Contains(t, result.Error(), "network timeout")

		var localErr *azdext.LocalError
		assert.False(t, errors.As(result, &localErr))
	})

	t.Run("nil error returns nil", func(t *testing.T) {
		assert.Nil(t, GraphError(nil, "ignored"))
	})
}
