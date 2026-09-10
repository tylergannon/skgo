Feature: A command handles every refresh its client requested

  A page asks for a single-flight refresh by writing `.updates(...)`, and kit's
  client posts the keys of the query instances it wants back. That list is
  client input: it is in the network tab, and anyone can post a longer one. So
  the server does not act on it — a command runs the queries its Go handler
  named, up to the number of instances it said it would accept. With SvelteKit
  3.0.0-next.27 it must explicitly ignore any requested query it deliberately
  leaves stale, or the client rejects the command response.

  `/todos/gate` is that rule with the lid off. `writeNotes` in
  `src/routes/todos/gate/gate.remote.go` changes both notes and the banner, and
  the page asks for all three back on every write. The handler names `getNote`
  and accepts one instance of it; it explicitly ignores `getBanner`.

  So of the three the page asks for: the left note comes back refreshed, the
  right note — the second instance of `getNote` — comes back refused with the
  reason on it, and the banner is deliberately not run and goes stale. Every text below is
  one this scenario typed into the page, and the reload button is what shows
  that a stale panel is a refresh that did not happen rather than a write that
  did not land.

  Scenario: The refresh the handler declared arrives in the command's own response
    Given I open "/todos/gate"
    And the gate page has loaded
    When I note the remote request count
    And I write left "hedgerows in the rain", right "a kettle on the hob" and banner "the tide is out"
    Then the panel "left" shows "hedgerows in the rain"
    And the command reported writing 3 values
    And exactly 1 remote request was made since
    And every part of the page loaded

  Scenario: A query the handler explicitly ignored is not run, and the write happened anyway
    Given I open "/todos/gate"
    And the gate page has loaded

    # A first write, then a reload, so every panel is showing a value this
    # scenario chose and has been seen on screen. Nothing below is measured
    # against the thing it is testing.
    When I write left "one lamp lit", right "two chairs empty" and banner "three miles to go"
    And I reload all three panels
    Then the panel "left" shows "one lamp lit"
    And the panel "right" shows "two chairs empty"
    And the panel "banner" shows "three miles to go"

    # A second write asks for the same three things and gets three different
    # answers.
    When I write left "four bells struck", right "five doors down" and banner "six weeks of frost"
    Then the panel "left" shows "four bells struck"
    And the panel "banner" still shows "three miles to go"
    And the command reported writing 3 values

    # And the banner did change on the server: what the page was showing was a
    # refresh the handler explicitly ignored, not a write that failed.
    When I reload all three panels
    Then the panel "banner" shows "six weeks of frost"

  Scenario: An instance past the handler's limit is refused, and says why
    Given I open "/todos/gate"
    And the gate page has loaded
    When I write left "seven swans", right "eight maids" and banner "nine drummers"
    Then the panel "left" shows "seven swans"
    And the panel "right" is refused with "A requested refresh of getNote was refused: this handler accepts at most 1"

    # Refused, not broken: the note is still there and still readable.
    When I reload all three panels
    Then the panel "right" shows "eight maids"
