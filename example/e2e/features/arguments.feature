Feature: A malformed argument is refused before the Go function runs

  A remote function's stub is declared `'unchecked'`, because there is no
  TypeScript schema to check it against: the Go signature is the schema, and
  `skgo generate` turns it into a strict decoder with polytype. So Go is the
  only thing standing between a browser and a Go function's argument.

  Every call below is made by the app's own page, at the app's own origin, to
  the same `/_app/remote/...` endpoint kit's client uses — so nothing about
  these arguments is out of reach of a real visitor with a console open. The
  page is still on screen when each one is refused, and the todo it named still
  reads what it read before.

  Scenario: A command missing a required field is refused and changes nothing
    Given I open "/todos"
    When the todo list has loaded
    And the page calls "renameTodo" with the argument [{"id":1},"t1"]
    Then the server refused it with 400 "Bad Request"
    And I see the todo "write the adapter"
    And every part of the page loaded

  Scenario: A command given a field of the wrong kind is refused and changes nothing
    Given I open "/todos"
    When the todo list has loaded
    And the page calls "renameTodo" with the argument [{"id":1,"text":2},"t1",7]
    Then the server refused it with 400 "Bad Request"
    And I see the todo "write the adapter"
    And every part of the page loaded

  Scenario: A command carrying a field the Go type does not have is refused
    Given I open "/todos"
    When the todo list has loaded
    And the page calls "renameTodo" with the argument [{"id":1,"text":2,"colour":3},"t1","renamed by a field nobody declared","green"]
    Then the server refused it with 400 "Bad Request"
    And I see the todo "write the adapter"
    And I do not see the todo "renamed by a field nobody declared"

  Scenario: A query declared without an argument refuses one
    Given I open "/"
    When the page asks "getSite" with the argument [null]
    Then the server refused it with 400 "Bad Request"
    And the site is named "skgo"
    And every part of the page loaded

  Scenario: The same command with a well-formed argument is answered
    Given I open "/todos"
    When the todo list has loaded
    And the page calls "renameTodo" with the argument [{"id":1,"text":2},"t1","write the adapter"]
    Then the server answered it
    And I see the todo "write the adapter"
    And every part of the page loaded
