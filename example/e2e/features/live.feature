Feature: A live query is rendered by Go and then keeps arriving

  `watchBoard` in src/routes/live/board.remote.go is a `query.live`. It pushes a
  board — how many todos this visitor may see, the text of the newest one, and
  which value of the stream this is — now and after every change, until the
  client disconnects. The generated src/routes/live/board.remote.ts throws
  "skgo: implemented in Go" in its body, so nothing on /live has a second way to
  appear.

  Two things have to be true at once, and they pull in opposite directions. The
  first value has to be *in the document*, rendered by Go before any script runs,
  which is what a live query awaited during a render means. And the values after
  it have to arrive on the stream the browser opens afterwards, without the page
  going back to ask for the first one again.

  "Stream frame" is the counter the Go producer keeps: 1 for the value the render
  was answered with, 2 for the first value pushed after the stream opened, and so
  on. A page that quietly refetched a static value instead of listening would sit
  on 1 for ever, which is the difference this feature is about.

  Scenario: The board is in the document Go sent, before any script runs
    Given another tab is open at "/todos"
    When the other tab adds the todo "the board arrived rendered"
    And I visit "/live"
    Then the document already said the board's newest todo is "the board arrived rendered"
    And the document already said the board is on stream frame 1
    And the document never mentions "skgo: implemented in Go"
    And every part of the page loaded

  Scenario: Hydration listens rather than refetching
    The browser opens the stream — that is what subscribing is — and it does
    nothing else. If the value had not travelled with the document, the board
    would arrive empty and be filled in by a second request; if the page had
    refetched it, there would be two.

    Given another tab is open at "/todos"
    When the other tab adds the todo "hydration listened"
    And I note the remote request count
    And I visit "/live"
    Then the document already said the board's newest todo is "hydration listened"
    And the board's newest todo is "hydration listened"
    And the board is on stream frame 1
    And exactly 1 remote request was made since
    And the only remote request since was the board's stream

  Scenario: A command in another tab reaches the open page over the stream
    Given I open "/live"
    And another tab is open at "/todos"
    And the board is on stream frame 1
    And the board's stream is open
    And I note the remote request count
    When the other tab adds the todo "a parcel from the other tab"
    Then the board's newest todo is "a parcel from the other tab"
    And the board is on stream frame 2
    And the board's count is the number of todos in the other tab
    And exactly 0 remote requests were made since
    And every part of the page loaded

  Scenario: Two changes are two pushes
    Given I open "/live"
    And another tab is open at "/todos"
    And the board is on stream frame 1
    When the other tab adds the todo "the first change"
    Then the board's newest todo is "the first change"
    And the board is on stream frame 2
    When the other tab adds the todo "the second change"
    Then the board's newest todo is "the second change"
    And the board is on stream frame 3
    And the board's count is the number of todos in the other tab
    And every part of the page loaded

  Scenario: The todos page carries its live count in the document too
    The count above the todo list is the same kind of function — a `query.live`
    awaited while the page renders — and it arrives the same way.

    Given I open "/todos"
    Then the document already carried the todo count
    And the todo count is the number of todos on the page
    And every part of the page loaded
