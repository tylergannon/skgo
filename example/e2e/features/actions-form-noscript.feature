Feature: A named Go page action works without JavaScript

  Scenario: A native POST saves Grace with scripting disabled
    Given I open "/"
    When I open the capability "Page form actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I save Grace with the native form
    Then the native response already contains the Grace receipt and saved profile
    And the Actions page shows the Grace receipt and saved profile
    And a later Actions GET retains the Grace profile

  Scenario: A native rejected edit retains fields with scripting disabled
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I submit an invalid Grace edit with the native form
    Then the native failure response already contains the retained fields and Ada profile
    And the Actions page retains the invalid Grace fields and Ada profile
    And a later Actions GET retains the Ada profile

  Scenario: A native archive has no receipt with scripting disabled
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I archive with the native form
    Then the native archive response already contains Ada Archived and null form data
    And the Actions page shows Ada Archived without a receipt
    And a later Actions GET retains Ada Archived

  Scenario: A native sign-in redirects and keeps its cookie without scripting
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I sign in as ada with native without JavaScript
    Then the submitting action redirects separately to the signed-in page
    And the destination reads the session cookie and greets ada
    And a later signed-in GET still greets ada

  Scenario: A native forbidden edit uses the Actions boundary without scripting
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I attempt a forbidden edit with native without JavaScript
    Then the submitting action reports the expected 403 error
    And the Actions boundary shows the permission error
    And returning from the error leaves the Ada fixture unchanged

  Scenario: A native unexpected failure stays safe without scripting
    Given I open "/actions"
    Then the Actions editor shows the Ada fixture
    And the Actions browser has JavaScript disabled
    When I trigger the demo service failure with native without JavaScript
    Then the submitting action reports a safe 500 error
    And the Actions boundary shows the public message and support ID
    And returning from the error leaves the Ada fixture unchanged
