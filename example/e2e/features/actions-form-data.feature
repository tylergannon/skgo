Feature: Go actions receive real browser form data

  Scenario Outline: A poem upload reaches Go through <submission>
    Given I open "/actions"
    When I send the haiku with <submission>
    Then Go reports haiku.txt, 72 bytes, its SHA-256 prefix, and both interests
    And the upload's Money survives the required browser path

    Examples:
      | submission            |
      | Kit enhancement       |
      | native then hydration |

  Scenario Outline: The clicked button controls <encoding> encoding with <submission>
    Given I open "/actions"
    When I submit the <encoding> encoding button with <submission>
    Then Go reports the chosen <encoding> button and ordered interests

    Examples:
      | encoding   | submission            |
      | standard   | Kit enhancement       |
      | multipart  | Kit enhancement       |
      | standard   | native then hydration |
      | multipart  | native then hydration |
