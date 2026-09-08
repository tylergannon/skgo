Feature: Independent Go operations overlap during server rendering

  Each query waits at a gate until three distinct operations have entered.
  A blocking bridge cannot return the fixture values, regardless of how fast
  the machine is. The request key separates each scenario's gate.

  Scenario: Independent queries finish together in the document
    Given I open a fresh "queries" concurrency page
    Then the document and hydrated page contain exactly "Amber, Birch, Cobalt"
    And hydration did not refetch the ordinary queries

  Scenario: Queries, a batch and a live first value overlap
    Given I open a fresh "mixed" concurrency page
    Then the document and hydrated page contain exactly "Amber, Birch one, Birch two, Cobalt"
    And the live query's browser reconnect settles without failing the page

  Scenario: A dependent query receives the first query's result
    Given I open a fresh "dependent" concurrency page
    Then the document and hydrated page show the expected harvest
    And hydration did not refetch the ordinary queries

  Scenario: Abandoned work finishes while a new page is rendering
    Given I cancel a document after its Go operation has started
    And I start another document while the abandoned operation is held
    When the abandoned operation finishes before the new operation
    Then the new document and hydrated page show only their own result
    And hydration did not refetch the ordinary queries
