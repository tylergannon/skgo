#ifndef SKGO_NATIVE_H
#define SKGO_NATIVE_H
#include <stddef.h>
#include <stdint.h>
typedef struct { const uint8_t *ptr; size_t len; } SKBytes;
typedef struct { uint8_t *ptr; size_t len; } SKBuffer;
typedef struct { SKBuffer url, origin, body, key; } SKRequest;
typedef struct { uint32_t kind, status; SKBuffer value, message; } SKReply;
// Inputs are borrowed for the synchronous call only. Outputs own independent
// buffers. Release each output buffer exactly once. Release zeroes the buffer.
// Status: 0 success, 1 allocation failure, 2 invalid input/wire, 3 unsupported,
// 4 cache capacity reached, 5 unretained query, 6 identifier exhaustion.
// Reply kind: 0 result, 1 HTTP error, 2 remote error, 3 Kit redirect.
// Arguments/results are finite-model JSON, not the remote wire format.
// A zero-length argument/value means omitted/undefined, distinct from JSON null.
uint32_t sk_prepare(SKBytes origin, SKBytes base, SKBytes id, uint32_t command, SKBytes argument, SKBytes refreshes, SKRequest *);
// Cache contexts are private to the Swift client implementation. Query keys and
// values cross as bytes; tickets are scalar completion identifiers, not objects.
// All operations, including reads and release, require exclusive access.
// refreshes/requested are JSON arrays of canonical key strings. A serial of 0
// from begin means no HTTP dispatch; returned snapshot buffers own their bytes.
typedef struct SKCache SKCache;
typedef struct { uint64_t epoch, serial; } SKTicket;
typedef struct { uint32_t ready, loading, status; SKBuffer value, message; } SKCacheState;
uint32_t sk_cache_create(uint32_t capacity, SKCache **);
void sk_cache_release(SKCache **); // releases and zeroes the private context
uint32_t sk_cache_retain_query(SKCache *, SKBytes key);
uint32_t sk_cache_release_query(SKCache *, SKBytes key);
uint32_t sk_cache_reset(SKCache *);
uint32_t sk_cache_begin(SKCache *, SKBytes key, uint32_t refresh, SKTicket *);
uint32_t sk_cache_waiting(SKCache *, SKBytes key, SKTicket);
void sk_cache_abort(SKCache *, SKBytes key, SKTicket);
uint32_t sk_cache_reject(SKCache *, SKBytes key, SKTicket, uint16_t status, SKBytes message);
uint32_t sk_cache_snapshot(SKCache *, SKBytes key, SKCacheState *);
uint32_t sk_cache_receive(SKCache *, SKBytes key, SKTicket, uint16_t status, SKBytes body, SKBytes requested, SKReply *);
uint32_t sk_decode(uint16_t http_status, SKBytes body, SKReply *);
void sk_buffer_release(SKBuffer *);
#endif
