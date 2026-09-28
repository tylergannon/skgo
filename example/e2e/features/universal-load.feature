Feature: Pure universal load over Go data

  Scenario: An invoice filter survives hydration and reruns through Kit navigation
    When I visit "/invoices?overdue=1"
    Then the invoice view shows the overdue fixture
    When I switch to all invoices through Kit navigation
    Then the invoice view shows the full fixture without another document or data request
