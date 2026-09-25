Feature: Go actions receive real browser form data without JavaScript

  Scenario: A native poem upload reaches Go with scripting disabled
    Given I open "/actions"
    Then the Actions browser has JavaScript disabled
    When I send the haiku with native form
    Then Go reports haiku.txt, 72 bytes, its SHA-256 prefix, and both interests
    And the upload's Money survives the required browser path

  Scenario Outline: A native clicked button controls <encoding> encoding without JavaScript
    Given I open "/actions"
    Then the Actions browser has JavaScript disabled
    When I submit the <encoding> encoding button with native form
    Then Go reports the chosen <encoding> button and ordered interests

    Examples:
      | encoding  |
      | standard  |
      | multipart |
