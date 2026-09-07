Feature: A batch query answers several components in one call

  `getQuotes` in src/routes/batch/quotes.remote.go is a `query.batch`. It is
  handed every symbol collected in one turn and answers them all at once, and
  each answer reports how many symbols were in the call that produced it. So a
  page whose four rows each asked for one symbol on their own should show four
  rows that each say "batch of 4": one call into Go, four arguments. If every
  call carried a single argument each row would say "batch of 1" — a page that
  works and a feature that does not.

  The prices are fixtures with one source each: SKGO 12.75, GOJA 34.60, KITX
  56.10 and SVLT 78.45 appear nowhere else in the app, and the generated
  src/routes/batch/quotes.remote.ts throws "skgo: implemented in Go".

  @prod
  Scenario Outline: The quotes are in the document, answered in one call of four
    Given I open "/batch"
    Then the document already said the quote for "<symbol>" is "<price>", answered in a call of 4
    And the quote for "<symbol>" is "<price>", answered in a call of 4

    Examples:
      | symbol | price |
      | SKGO   | 12.75 |
      | GOJA   | 34.60 |
      | KITX   | 56.10 |
      | SVLT   | 78.45 |

  @prod
  Scenario: Hydration does not refetch what the batch already answered
    Given I note the remote request count
    When I visit "/batch"
    Then the quote for "SKGO" is "12.75", answered in a call of 4
    And the quote for "SVLT" is "78.45", answered in a call of 4
    And exactly 0 remote requests were made since
    And every part of the page loaded

  Scenario: A client-side visit sends the four calls as one request
    Nothing is rendered by Go here: the browser is already on another page and
    navigates to this one itself. Four components each ask for one symbol, and
    "batch of 4" on every row is the server saying it was asked all four at once.

    The count is of calls to this endpoint rather than of remote calls in
    general, because in dev the page the browser started on is still fetching
    its own values when this scenario begins — traffic that belongs to another
    feature. Four calls answered by one request is the whole claim, and a batch
    that had stopped batching would send four.

    Given I open "/"
    And I note the remote request count
    When I click the link to "/batch"
    Then the quote for "SKGO" is "12.75", answered in a call of 4
    And the quote for "GOJA" is "34.60", answered in a call of 4
    And the quote for "KITX" is "56.10", answered in a call of 4
    And the quote for "SVLT" is "78.45", answered in a call of 4
    And the quotes endpoint was asked exactly once since
    And every part of the page loaded
