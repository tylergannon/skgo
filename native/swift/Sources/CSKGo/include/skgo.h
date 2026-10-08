#ifndef SKGO_NATIVE_H
#define SKGO_NATIVE_H
#include <stddef.h>
#include <stdint.h>
typedef struct { const uint8_t *ptr; size_t len; } SKBytes;
typedef struct { uint8_t *ptr; size_t len; } SKBuffer;
typedef struct { SKBuffer url, origin, body; } SKRequest;
typedef struct { uint32_t kind, status; SKBuffer value, message; } SKReply;
// Inputs are borrowed for the synchronous call only. Outputs own independent
// buffers. Release each output buffer exactly once. Release zeroes the buffer.
// Status: 0 success, 1 allocation failure, 2 invalid input/wire, 3 unsupported.
// Reply kind: 0 result, 1 HTTP error, 2 remote error, 3 Kit redirect.
// Arguments/results are finite-model JSON, not the remote wire format.
// A zero-length argument/value means omitted/undefined, distinct from JSON null.
uint32_t sk_prepare(SKBytes origin, SKBytes base, SKBytes id, uint32_t command, SKBytes argument, SKRequest *);
uint32_t sk_decode(uint16_t http_status, SKBytes body, SKReply *);
void sk_buffer_release(SKBuffer *);
#endif
