public enum NativeModelError: Error, Sendable {
    case invalid(String)
}
fileprivate enum _SKGoWire {
    static func depth(_ value: Int) throws {
        guard value < 256 else { throw NativeModelError.invalid("Model depth exceeds 255") }
    }
    static func object(_ value: Any, keys: Set<String>) throws -> [String: Any] {
        guard let object = value as? [String: Any], Set(object.keys).isSubset(of: keys) else {
            throw NativeModelError.invalid("Expected a closed model object")
        }
        return object
    }
    static func required(_ object: [String: Any], _ key: String) throws -> Any {
        guard let value = object[key] else { throw NativeModelError.invalid("Missing required field: \(key)") }
        return value
    }
    static func optional<T>(_ value: Any?, decode: (Any) throws -> T) throws -> T? {
        guard let value else { return nil }
        return try decode(value)
    }
    static func nullable<T>(_ value: Any, decode: (Any) throws -> T) throws -> T? {
        if value is NSNull { return nil }
        return try decode(value)
    }
    static func string(_ value: Any) throws -> String {
        guard let string = value as? String else { throw NativeModelError.invalid("Expected a string") }
        return string
    }
    static func boolean(_ value: Any) throws -> Bool {
        guard let number = value as? NSNumber, CFGetTypeID(number) == CFBooleanGetTypeID() else {
            throw NativeModelError.invalid("Expected a boolean")
        }
        return number.boolValue
    }
    static func numeric(_ value: Any) throws -> Double {
        guard let number = value as? NSNumber, CFGetTypeID(number) != CFBooleanGetTypeID(), number.doubleValue.isFinite else {
            throw NativeModelError.invalid("Expected a finite number")
        }
        return number.doubleValue
    }
    static func integer<T: FixedWidthInteger>(_ value: Any, as type: T.Type) throws -> T {
        let number = try numeric(value)
        guard number.rounded(.towardZero) == number, abs(number) <= 9_007_199_254_740_991, let result = T(exactly: number) else {
            throw NativeModelError.invalid("Integer is outside its native/wire domain")
        }
        return result
    }
    static func number<T: BinaryFloatingPoint>(_ value: Any, as type: T.Type) throws -> T {
        let result = T(try numeric(value))
        guard result.isFinite else { throw NativeModelError.invalid("Number is outside its native domain") }
        return result
    }
    static func encodeInteger<T: BinaryInteger>(_ value: T) throws -> Any {
        guard let number = Int64(exactly: value), number >= -9_007_199_254_740_991, number <= 9_007_199_254_740_991 else {
            throw NativeModelError.invalid("Integer is outside the JavaScript number domain")
        }
        return NSNumber(value: number)
    }
    static func encodeNumber<T: BinaryFloatingPoint>(_ value: T) throws -> Any {
        let number = Double(value)
        guard number.isFinite else { throw NativeModelError.invalid("Expected a finite number") }
        return NSNumber(value: number)
    }
    static func array(_ value: Any, length: Int? = nil) throws -> [Any] {
        guard let array = value as? [Any], length == nil || array.count == length else {
            throw NativeModelError.invalid("Expected an array with the declared length")
        }
        return array
    }
    static func encodeArray<T>(_ values: [T], length: Int? = nil, encode: (T) throws -> Any) throws -> Any {
        guard length == nil || values.count == length else { throw NativeModelError.invalid("Incorrect fixed array length") }
        return try values.map(encode)
    }
}
