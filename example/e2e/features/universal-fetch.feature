Feature: A universal load's fetches are replayed into the page, not repeated

  A page's universal load fetches the app's own endpoints while the document is
  rendered, and again in the browser. Kit puts what the first run read in the
  document so the second finds it there. The numbers below are the number of
  times the server's endpoints were really called, fixed in the scenarios; the
  same claims run against the built bundle and `vp dev`.

  Scenario: A cold document carries every answer and Kit's client hydrates without asking again
    Given I use a fresh replay run
    When I visit the universal fetch page at step 1
    Then the document already said these values:
      | testid         | text                                                                  |
      | uf-text        | plain-lantern-7                                                       |
      | uf-json        | harbour@1                                                             |
      | uf-aliased     | harbour@rewritten                                                     |
      | uf-binary      | 00ff10807fc328                                                        |
      | uf-shown       | shown-7                                                               |
      | uf-streamed    | alpha-beta-gamma                                                      |
      | uf-empty       | 204:none                                                              |
      | uf-missing     | 404:false:no such lamp                                                |
      | uf-header-a    | a@1                                                                   |
      | uf-header-b    | b@1                                                                   |
      | uf-body-alpha  | POST:alpha@1                                                          |
      | uf-body-beta   | POST:beta@1                                                           |
      | uf-form        | POST:lamp=one:application/x-www-form-urlencoded;charset=UTF-8         |
      | uf-cached      | cached                                                                |
      | uf-nested      | harbour-lamp-4096                                                     |
    And the page shows these values:
      | testid         | text                                                                  |
      | uf-binary      | 00ff10807fc328                                                        |
      | uf-shown       | shown-7                                                               |
      | uf-header-a    | a@1                                                                   |
      | uf-header-b    | b@1                                                                   |
      | uf-body-alpha  | POST:alpha@1                                                          |
      | uf-body-beta   | POST:beta@1                                                           |
      | uf-nested      | harbour-lamp-4096                                                     |
    And neither the document nor the page carries "hidden-value-31"
    And the replay endpoints have been called exactly:
      | request                   | count |
      | GET /universal-fetch      | 1     |
      | GET /api/replay/text      | 1     |
      | GET /api/replay/json      | 2     |
      | GET /api/replay/binary    | 1     |
      | GET /api/replay/stream    | 1     |
      | GET /api/replay/breaking  | 1     |
      | GET /api/replay/empty     | 1     |
      | GET /api/replay/missing   | 1     |
      | GET /api/replay/echo      | 2     |
      | POST /api/replay/echo     | 2     |
      | POST /api/replay/form     | 2     |
      | GET /api/replay/cached    | 1     |
      | GET /request-fetch        | 1     |

  Scenario: A Kit navigation shows the new values and asks the endpoints exactly once more, except for what may be kept
    Given I use a fresh replay run
    When I visit the universal fetch page at step 1
    And I follow the universal fetch next step link
    # `handleFetch` is a server hook: the browser's own request for the aliased URL
    # reaches Go as written, so its answer has no step, where the server's was rewritten.
    Then the page shows these values:
      | testid         | text                                                                  |
      | step           | 2                                                                     |
      | uf-text        | plain-lantern-7                                                       |
      | uf-json        | harbour@2                                                             |
      | uf-aliased     | harbour@                                                              |
      | uf-binary      | 00ff10807fc328                                                        |
      | uf-shown       | shown-7                                                               |
      | uf-streamed    | alpha-beta-gamma                                                      |
      | uf-empty       | 204:none                                                              |
      | uf-missing     | 404:false:no such lamp                                                |
      | uf-header-a    | a@2                                                                   |
      | uf-header-b    | b@2                                                                   |
      | uf-body-alpha  | POST:alpha@2                                                          |
      | uf-body-beta   | POST:beta@2                                                           |
      | uf-form        | POST:lamp=one:application/x-www-form-urlencoded;charset=UTF-8         |
      | uf-cached      | cached                                                                |
      | uf-nested      | harbour-lamp-4096                                                     |
    And neither the document nor the page carries "hidden-value-31"
    And the replay endpoints have been called exactly:
      | request                   | count |
      | GET /universal-fetch      | 1     |
      | GET /api/replay/text      | 2     |
      | GET /api/replay/json      | 4     |
      | GET /api/replay/binary    | 2     |
      | GET /api/replay/stream    | 2     |
      | GET /api/replay/breaking  | 2     |
      | GET /api/replay/empty     | 2     |
      | GET /api/replay/missing   | 2     |
      | GET /api/replay/echo      | 4     |
      | POST /api/replay/echo     | 4     |
      | POST /api/replay/form     | 3     |
      | GET /api/replay/cached    | 1     |
      | GET /request-fetch        | 2     |
