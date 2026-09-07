Feature: A Go slice that was never filled is still a list

  Go writes `nil` for a slice nobody put anything in, and that is how a
  handler says "no rows". The TypeScript skgo generates for the same field
  says `Array<T>`, with no null in it, so the page reads `.length` and walks
  the list without asking whether it is there.

  Every count on /empty is read off the value Go sent. A value that arrived
  as null cannot be counted at all: the page would throw and show its failure
  panel instead of a list, which is what the "every part of the page loaded"
  step below refuses.

  Background:
    Given I open "/empty"

  Scenario: A report with no diagnostics shows an empty list, not a broken page
    Then every part of the page loaded
    And the report is titled "Parse failed"
    And the report shows 0 diagnostics and the empty state "No diagnostics."
    And the models list shows 0 models and the empty state "No models."
    And the load's notes show 0 notes and the empty state "No notes."

  # The control, on the same page and through the same encoder. Without it,
  # every claim above would be satisfied by a page that rendered nothing.
  Scenario: The same page still renders a list that does have rows in it
    Then every part of the page loaded
    And the report titled "Parsed" lists "unused import fmt" and "missing return"

  @prod
  Scenario: The empty list was already in the document Go sent
    Then the document response came from skgo in the expected mode
    And the document already said '<p data-testid="report-count">0 diagnostics</p>'
    And the document already said '<li data-testid="report-empty">No diagnostics.</li>'
    And the document already said '<p data-testid="notes-count">0 notes</p>'
    And every part of the page loaded

  Scenario: A client-side navigation to the page shows the same empty list
    When I click the link to "/"
    And I click the link to "/empty"
    Then exactly 1 document request was made
    And every part of the page loaded
    And the report shows 0 diagnostics and the empty state "No diagnostics."
    And the models list shows 0 models and the empty state "No models."

  Scenario: A command's result counts the same way
    When I press reparse
    Then the reparse result reads "Reparsed: 0 diagnostics"
    And every part of the page loaded
