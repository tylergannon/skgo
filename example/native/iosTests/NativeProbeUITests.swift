import XCTest

final class NativeProbeUITests: XCTestCase {
    @MainActor
    func testNativeCallsAndLocalPageAcrossBackground() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launch()
        let call = app.buttons["Run native query and command"]
        XCTAssertTrue(call.waitForExistence(timeout: 30))
        let page = app.webViews.firstMatch
        XCTAssertTrue(page.waitForExistence(timeout: 30))
        XCTAssertTrue(page.staticTexts["Todos"].waitForExistence(timeout: 30))
        call.tap()
        XCTAssertTrue(app.staticTexts["Command: Native probe; query: Native probe"].waitForExistence(timeout: 30))

        XCUIDevice.shared.press(.home)
        let background = XCTNSPredicateExpectation(
            predicate: NSPredicate { _, _ in
                app.state == .runningBackground || app.state == .runningBackgroundSuspended
            }, object: app)
        XCTAssertEqual(XCTWaiter.wait(for: [background], timeout: 10), .completed)
        app.activate()
        XCTAssertTrue(app.wait(for: .runningForeground, timeout: 10))
        XCTAssertTrue(call.waitForExistence(timeout: 30))
        XCTAssertTrue(page.staticTexts["Todos"].waitForExistence(timeout: 30))
        call.tap()
        XCTAssertTrue(app.staticTexts["Command: Native probe; query: Native probe"].waitForExistence(timeout: 30))
    }
}
