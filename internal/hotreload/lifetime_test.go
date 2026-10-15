package hotreload

// Kernel observations are made in the native host against its actual code/data
// addresses. No loader live-count or reported unload success is the oracle.
type nativeRange struct {
	Base, Size uintptr
	Mapped     bool
}

const lifetimeProbe = `package orders
import("fmt";"os";"reflect";"runtime";"syscall";"unsafe";"github.com/ebitengine/purego")
type SkgoNativeRange struct{Base,Size uintptr;Mapped bool}
var nativeMincore func(uintptr,uintptr,*byte) int32
var nativeErrno func() *int32
var nativeRegion func(uint32,*uint64,*uint64,int32,*int32,*uint32,*uint32) int32
var nativeDeallocate func(uint32,uint32) int32
var nativeTask uint32
func init(){
 library,errno:="libc.so.6","__errno_location";if runtime.GOOS=="darwin"{library,errno="/usr/lib/libSystem.B.dylib","__error"}
 handle,err:=purego.Dlopen(library,purego.RTLD_NOW|purego.RTLD_GLOBAL);if err!=nil{panic(err)}
 purego.RegisterLibFunc(&nativeMincore,handle,"mincore");purego.RegisterLibFunc(&nativeErrno,handle,errno)
 if runtime.GOOS=="darwin" {
  purego.RegisterLibFunc(&nativeRegion,handle,"mach_vm_region")
  purego.RegisterLibFunc(&nativeDeallocate,handle,"mach_port_deallocate")
  address,err:=purego.Dlsym(handle,"mach_task_self_");if err!=nil{panic(err)};nativeTask=*(*uint32)(unsafe.Pointer(address))
 }
}
func SkgoHotProbe(ranges []SkgoNativeRange) []SkgoNativeRange {
 _skgo_mu.Lock();defer _skgo_mu.Unlock();runtime.GC()
 return nativeProbe(ranges)
}
func nativeProbe(ranges []SkgoNativeRange) []SkgoNativeRange {
 runtime.LockOSThread();defer runtime.UnlockOSThread()
 for i:=range ranges {
  r:=&ranges[i];mapped,missing:=false,false
  for off:=uintptr(0);off<r.Size;off+=uintptr(os.Getpagesize()) {
   if runtime.GOOS=="darwin" {
    address:=uint64(r.Base+off);var size uint64;var info [9]int32;count:=uint32(len(info));var object uint32
    status:=nativeRegion(nativeTask,&address,&size,9,&info[0],&count,&object)
    if object!=0 {if status:=nativeDeallocate(nativeTask,object);status!=0{panic(fmt.Sprintf("mach_port_deallocate: %d",status))}}
    if status!=0 && status!=1 {panic(fmt.Sprintf("mach_vm_region: %d",status))}
    if status==1||address>uint64(r.Base+off) {missing=true}else{mapped=true}
    continue
   }
   var residency byte
   if nativeMincore(r.Base+off,uintptr(os.Getpagesize()),&residency)!=0 {
    if *nativeErrno()!=int32(syscall.ENOMEM){panic(fmt.Sprintf("mincore(%x): errno %d",r.Base+off,*nativeErrno()))}
    missing=true
   } else {mapped=true}
  }
  if mapped&&missing {panic(fmt.Sprintf("partially mapped native range: %+v",r))}
  r.Mapped=mapped
 }
 return ranges
}
func SkgoHotRanges() []SkgoNativeRange {
 _skgo_mu.Lock();defer _skgo_mu.Unlock()
 module:=reflect.ValueOf(_skgo_current.Load()._skgo_revision.module).Elem()
 var ranges []SkgoNativeRange
 for _,segment:=range []struct{field,base string}{{"codeSeg","codeBase"},{"dataSeg","dataBase"}} {
  value:=module.FieldByName(segment.field)
  ranges=append(ranges,SkgoNativeRange{Base:uintptr(value.FieldByName(segment.base).Int()),Size:uintptr(value.FieldByName("maxLen").Int())})
 }
 return nativeProbe(ranges)
}
func SkgoHotSaved()(int,uintptr) {
 if service.callback==nil{return 0,0}
 return service.callback(),reflect.ValueOf(service.callback).Pointer()
}
`
