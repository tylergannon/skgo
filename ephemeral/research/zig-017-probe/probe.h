#include <stddef.h>
#include <stdint.h>
typedef struct Snapshot Snapshot;
typedef struct { const uint8_t *ptr; size_t len; } Bytes;
Snapshot *snapshot_new(const uint8_t *ptr, size_t len);
void snapshot_retain(Snapshot *s);
void snapshot_release(Snapshot *s);
Bytes snapshot_bytes(const Snapshot *s);
