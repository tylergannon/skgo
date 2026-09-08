Feature: A load can promise several values, and each one arrives when it is ready

  A load may hand back values it does not have yet. The page is sent at once,
  with its own loading states already in it, and each value is appended to the
  same response the moment it is ready — no second request, and nobody waits for
  the slowest part of a page before they can read the rest of it.

  The fixture is src/routes/stream/page.server.go. It promises three things and
  makes each of them take a different amount of time:

    * the ticker, "the first thing to arrive", after 0.8s
    * the digest, "the second thing to arrive" and "and the rest of the digest",
      after 2.2s
    * the forecast, "the last thing to arrive", after 3.6s

  The forecast is not a field of what the load returned — it sits inside the
  weather panel, one level down, which is where kit allows a promise to sit: its
  serializer finds one with a devalue reducer and its client reads one back with
  a devalue reviver, and both walk the whole tree.

  The three strings appear nowhere else in the app, and the generated TypeScript
  load beside the Go one throws "skgo: implemented in Go", so none of them has a
  second way to reach the page.

  The numbering is the point of the ordering claims. Kit numbers a promise when
  it serializes the page, in the order devalue walks the load's result — and
  devalue sorts an object's keys, so the digest is numbered 1, the ticker 2 and
  the forecast 3. They arrive in the other order. A response that carried them
  as 1, 2, 3 would be holding a value that was ready behind one that was not,
  which is the thing kit's own streaming helper exists to avoid.

  Scenario: A cold load shows three loading states and replaces them one at a time
    When I start loading "/stream"
    Then the page says "Three promises, one response" and is waiting for all three values
    When every promised value has arrived
    Then the ticker says "the first thing to arrive"
    And the digest reads "the second thing to arrive" and "and the rest of the digest"
    And the forecast says "the last thing to arrive"

  Scenario: The document carries the loading states and the values follow it, as they settle
    When I start loading "/stream"
    And every promised value has arrived
    Then the document held all three loading states and none of the three values
    And the document was followed by these values, in this order
      | promise | value                      |
      | 2       | the first thing to arrive  |
      | 1       | the second thing to arrive |
      | 3       | the last thing to arrive   |

  Scenario: A client-side navigation fills the page in exactly as a cold load does
    There is no document here. The browser is already running kit's client, so
    it asks for the page's data and reads it as it arrives — and the page has to
    behave the same way it does on a cold load, right down to which value shows
    up first.

    Given I open "/about"
    And the app says it is deployed as "skgo example"
    And I note the data request count
    When I follow the "Stream" link
    Then the page says "Three promises, one response" and is waiting for all three values
    And exactly 1 data request was made since
    When the ticker arrives
    Then the ticker says "the first thing to arrive" while the digest and forecast are still pending
    When every promised value has arrived
    Then the digest reads "the second thing to arrive" and "and the rest of the digest"
    And the forecast says "the last thing to arrive"
    And that data response, asked for again, named three promises and carried none of their values
    And it carried these values, in this order
      | promise | value                      |
      | 2       | the first thing to arrive  |
      | 1       | the second thing to arrive |
      | 3       | the last thing to arrive   |

  Rule: What the rendering engine reports reaches the server's log

    The engine is a bare ECMAScript runtime inside the Go process. Kit and
    Svelte both report a render-time failure through `console` — kit's
    `log_handle_error_hook_failure` and Svelte's `unresolved_hydratable` call it
    outright, in a production build — and there was no such global, so every one
    of those reports was a ReferenceError thrown in the middle of the render.
    The page fell to the error boundary and the log said nothing at all.

    src/routes/console/+page.svelte is a page that reports the same way an app
    would. The claim has two halves and needs both: the page renders, and the
    report comes out where an operator will see it.

    Scenario: A page that reports a failure while it renders still renders, and says so in the log
      Given I note where the server's log has got to
      When I visit "/console"
      Then I see "Console"
      And the page says it reported a failure while it rendered
      And the server's log has since carried "skgo-console-probe: this page reported a failure while rendering"
