// Package remoteprotocol exposes Kit remote argument framing for generated
// clients. It shares the server's encoding, including query canonicalization.
package remoteprotocol

import "github.com/tylergannon/skgo/internal/remotearg"

// QueryArgument returns Kit's canonical unpadded base64url argument.
// Pass devalue.Undefined for a function with no input; nil means null.
func QueryArgument(value any) (string, error) { return remotearg.StringifyQueryArg(value) }

// CommandArgument preserves ordered devalue objects rather than sorting them.
func CommandArgument(value any) (string, error) { return remotearg.StringifyCommandArg(value) }
