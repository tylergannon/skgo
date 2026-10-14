package main

/*
#include <mach/mach.h>
#include <mach/mach_vm.h>

// mach_vm_region returns the next region when the requested address is a hole.
static int mapped_at(unsigned long long target) {
 mach_vm_address_t address = target;
 mach_vm_size_t size = 0;
 vm_region_basic_info_data_64_t info;
 mach_msg_type_number_t count = VM_REGION_BASIC_INFO_COUNT_64;
 mach_port_t object = MACH_PORT_NULL;
 kern_return_t result = mach_vm_region(mach_task_self(), &address, &size,
  VM_REGION_BASIC_INFO_64, (vm_region_info_t)&info, &count, &object);
 if (object != MACH_PORT_NULL) mach_port_deallocate(mach_task_self(), object);
 if (result == KERN_INVALID_ADDRESS) return 0;
 if (result != KERN_SUCCESS) return -1;
 return address <= target && target < address + size;
}
*/
import "C"

// mappedAt is diagnostic OS observation for the POC tests. The experimental
// loader already depends on cgo; this must not enter SKGO's production runtime.
func mappedAt(address uintptr) int { return int(C.mapped_at(C.ulonglong(address))) }
