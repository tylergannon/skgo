Feature: A named Go page action saves a profile

  Each browser context starts with its own Ada workspace. The receipt comes
  from Kit's form channel; the saved profile comes from a Go page load.

  Scenario: Kit enhancement saves Grace without replacing the document
    Given I open "/"
    When I open the capability "Page form actions"
    Then the Actions editor shows the Ada fixture
    When I save Grace with Kit enhancement
    Then Kit receives a typed save result without a new document
    And the Actions page shows the Grace receipt and saved profile
    And a later Actions GET retains the Grace profile

  Scenario: A native POST renders the receipt before hydration and keeps it afterward
    Given I open "/"
    When I open the capability "Page form actions"
    Then the Actions editor shows the Ada fixture
    When I save Grace with the native form
    Then the native response already contains the Grace receipt and saved profile
    And the Actions page shows the Grace receipt and saved profile
    When I exercise the hydrated Actions client
    Then the Grace receipt and saved profile survive hydration
    And a later Actions GET retains the Grace profile

  Scenario: Kit enhancement retains a rejected edit and the Ada profile
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I submit an invalid Grace edit with Kit enhancement
    Then Kit receives a validation failure without a new document
    And the Actions page retains the invalid Grace fields and Ada profile
    And a later Actions GET retains the Ada profile

  Scenario: A native rejected edit is complete before and after hydration
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I submit an invalid Grace edit with the native form
    Then the native failure response already contains the retained fields and Ada profile
    And the Actions page retains the invalid Grace fields and Ada profile
    When I exercise the hydrated Actions client
    Then the retained failure and Ada profile survive hydration
    And a later Actions GET retains the Ada profile

  Scenario: Kit enhancement archives without action data
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I archive with Kit enhancement
    Then Kit receives a no-data success without a new document
    And the Actions page shows Ada Archived without a receipt
    And a later Actions GET retains Ada Archived

  Scenario: A native archive has null form data before and after hydration
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I archive with the native form
    Then the native archive response already contains Ada Archived and null form data
    And the Actions page shows Ada Archived without a receipt
    When I exercise the hydrated Actions client
    Then Ada Archived without a receipt survives hydration
    And a later Actions GET retains Ada Archived

  Scenario Outline: Sign-in redirects with a persistent session cookie through <submission>
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I sign in as ada with <submission>
    Then the submitting action redirects separately to the signed-in page
    And the destination reads the session cookie and greets ada
    When I exercise the hydrated Actions client
    Then the signed-in greeting survives hydration
    And a later signed-in GET still greets ada

    Examples:
      | submission            |
      | Kit enhancement       |
      | native then hydration |

  Scenario Outline: A forbidden edit uses the Actions error boundary through <submission>
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I attempt a forbidden edit with <submission>
    Then the submitting action reports the expected 403 error
    And the Actions boundary shows the permission error
    When I exercise the hydrated Actions client
    Then the permission error survives hydration
    And returning from the error leaves the Ada fixture unchanged

    Examples:
      | submission            |
      | Kit enhancement       |
      | native then hydration |

  Scenario Outline: An unexpected failure keeps its private detail out through <submission>
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    When I trigger the demo service failure with <submission>
    Then the submitting action reports a safe 500 error
    And the Actions boundary shows the public message and support ID
    When I exercise the hydrated Actions client
    Then the safe failure survives hydration
    And returning from the error leaves the Ada fixture unchanged

    Examples:
      | submission            |
      | Kit enhancement       |
      | native then hydration |
