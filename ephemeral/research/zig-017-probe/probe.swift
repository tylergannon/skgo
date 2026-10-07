import Foundation

var input = Data(repeating: 0x41, count: 4096)
let handle = input.withUnsafeBytes { bytes in
    snapshot_new(bytes.baseAddress!.assumingMemoryBound(to: UInt8.self), bytes.count)!
}
input.removeAll()
snapshot_retain(handle)
let borrowed = snapshot_bytes(handle)
snapshot_release(handle) // release initial/cache owner; the retained owner survives
precondition(borrowed.len == 4096 && borrowed.ptr![0] == 0x41 && borrowed.ptr![4095] == 0x41)
var freed = false
var wrapped: Data? = Data(
    bytesNoCopy: UnsafeMutableRawPointer(mutating: borrowed.ptr!),
    count: Int(borrowed.len),
    deallocator: .custom { _, _ in snapshot_release(handle); freed = true }
)
wrapped!.withUnsafeBytes { bytes in
    precondition(bytes.baseAddress == UnsafeRawPointer(borrowed.ptr!))
    precondition(bytes[0] == 0x41 && bytes[4095] == 0x41)
}
wrapped = nil
precondition(freed)
print("PASS: Swift scoped input; retained Zig arena; unchanged borrowed address; Data custom release")
