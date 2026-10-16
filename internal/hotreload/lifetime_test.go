package hotreload

// Kernel observations are made in the native host against its actual code/data
// addresses. No loader live-count or reported unload success is the oracle.
type nativeRange struct {
	Base, Size                                     uintptr
	Object, Offset, ObservedObject, ObservedOffset uint64
	Mapped                                         bool
}

const lifetimeProbe = `package orders
import("fmt";"os";"reflect";"runtime";"syscall";"unsafe";"github.com/ebitengine/purego")
type SkgoNativeRange struct{Base,Size uintptr;Object,Offset,ObservedObject,ObservedOffset uint64;Mapped bool}
var nativeMincore func(uintptr,uintptr,*byte) int32
var nativeErrno func() *int32
var nativeRegion func(uint32,*uint64,*uint64,*uint32,*int32,*uint32) int32
var nativeTask uint32
func init(){
 library,errno:="libc.so.6","__errno_location";if runtime.GOOS=="darwin"{library,errno="/usr/lib/libSystem.B.dylib","__error"}
 handle,err:=purego.Dlopen(library,purego.RTLD_NOW|purego.RTLD_GLOBAL);if err!=nil{panic(err)}
 purego.RegisterLibFunc(&nativeMincore,handle,"mincore");purego.RegisterLibFunc(&nativeErrno,handle,errno)
 if runtime.GOOS=="darwin" {
  purego.RegisterLibFunc(&nativeRegion,handle,"mach_vm_region_recurse")
  address,err:=purego.Dlsym(handle,"mach_task_self_");if err!=nil{panic(err)};nativeTask=*(*uint32)(unsafe.Pointer(address))
 }
}
func SkgoHotProbe(ranges []SkgoNativeRange) []SkgoNativeRange {
 _skgo_mu.Lock();defer _skgo_mu.Unlock()
 module:=reflect.ValueOf(_skgo_current.Load()._skgo_revision.module).Elem().FieldByName("module").Elem()
 it:=module.FieldByName("typemap").MapRange()
 for it.Next(){ptr:=it.Value().Pointer();for _,old:=range ranges{if ptr>=old.Base&&ptr<old.Base+old.Size{
  fmt.Fprintf(os.Stderr,"successor typemap pointer %x references predecessor range %x+%x\n",ptr,old.Base,old.Size);os.Exit(99)
 }}}
 runtime.GC()
 return nativeProbe(ranges)
}
func nativeProbe(ranges []SkgoNativeRange) []SkgoNativeRange {
 runtime.LockOSThread();defer runtime.UnlockOSThread()
 for i:=range ranges {
  r:=&ranges[i];mapped,missing:=false,false
  for off:=uintptr(0);off<r.Size;off+=uintptr(os.Getpagesize()) {
   if runtime.GOOS=="darwin" {
    address:=uint64(r.Base+off);var size uint64;var info [21]int32;count:=uint32(len(info));depth:=^uint32(0)
    status:=nativeRegion(nativeTask,&address,&size,&depth,&info[0],&count)
    if status==0 && info[12]!=0 {panic("mach_vm_region_recurse returned a submap")}
    if status!=0 && status!=1 {panic(fmt.Sprintf("mach_vm_region_recurse: %d",status))}
    if status==1||address>uint64(r.Base+off) {missing=true}else{
     // macOS may immediately reuse an unmapped address for unrelated backing.
     // Compare the original kernel object, not permanent address vacancy.
     object:=uint64(uint32(info[17]))|uint64(uint32(info[18]))<<32
     if object==0 {panic("native mapping has no kernel object identity")}
     offset:=uint64(uint32(info[3]))|uint64(uint32(info[4]))<<32;offset+=uint64(r.Base+off)-address
     r.ObservedObject,r.ObservedOffset=object,offset
     if r.Object==0 {r.Object=object;r.Offset=offset}
     if r.Object==object {mapped=true}else{missing=true}
    }
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
