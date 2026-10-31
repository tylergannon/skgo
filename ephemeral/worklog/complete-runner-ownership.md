decision: Preserve the existing build and qualification process while completing runner ownership migration. Qualify the existing release jobs without publishing a new product release.

decision: User authorized review/merge of #438 and real release-job certification on Bob and Amritanandamayi. Refresh the branch against current main, then dispatch Release from merged main using an existing published tag and publish=false; the release environment permits main and version tags, so a branch run cannot certify signed Mac packaging.
friction: The independent agent CLI failed with expired Claude OAuth (session 21160a9a-6442-47e6-8ad9-f528df3da753); an independent review agent completed the review with no findings instead. This is separate from the repaired GitHub CLI login on Amritanandamayi.
