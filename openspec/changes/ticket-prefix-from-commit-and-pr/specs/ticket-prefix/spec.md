## ADDED Requirements

### Requirement: Work-item id resolution from multiple sources

When `ticket_prefix_pattern` is configured, the gate SHALL resolve a work-item id by matching that pattern against a fixed ordered list of sources and using the first source that yields a match.
The resolution order SHALL be: the branch name, then the PR title when a pull request exists for the run, then the first commit subject on the branch not authored by the gate (oldest first) that carries a match.
Commit subjects authored by the gate itself SHALL NOT be treated as author commit subjects for resolution.
When `ticket_prefix_pattern` is empty, or no source yields a match, resolution SHALL yield no id.

#### Scenario: Ticket only on the branch name
- **WHEN** the pattern is `WEB-\d+` and the branch is `WEB-12345-fix-thing`
- **THEN** the resolved id is `WEB-12345`

#### Scenario: Ticket on the author commit but not the branch
- **WHEN** the pattern is `WEB-\d+`, the branch is `fix/jira-empty-release`, and the original author commit subject is `WEB-12345: fix empty releases`
- **THEN** the resolved id is `WEB-12345`

#### Scenario: Ticket only on the PR title
- **WHEN** the pattern is `WEB-\d+`, neither the branch nor any author commit subject matches, and the PR title is `WEB-12345: fix empty releases`
- **THEN** the resolved id is `WEB-12345`

#### Scenario: Branch takes precedence over PR title and commit
- **WHEN** the branch matches `WEB-100`, the PR title matches `WEB-200`, and an author commit subject matches `WEB-300`
- **THEN** the resolved id is `WEB-100`

#### Scenario: PR title takes precedence over the commit
- **WHEN** the branch carries no match, the PR title matches `WEB-200`, and an author commit subject matches `WEB-300`
- **THEN** the resolved id is `WEB-200`

#### Scenario: No source carries a ticket
- **WHEN** the pattern is `WEB-\d+` and none of the branch, author commit subjects, or PR title contains a match
- **THEN** resolution yields no id

#### Scenario: Pattern not configured
- **WHEN** `ticket_prefix_pattern` is empty
- **THEN** resolution yields no id regardless of branch, commits, or PR title

### Requirement: Gate-authored commit subjects lead with the resolved id

When a work-item id is resolved, the gate SHALL prepend `<id>: ` to the commit subject it would otherwise have written, keeping the rest of that subject verbatim.
Step fix commits SHALL therefore read `<id>: no-mistakes(<step>): <summary>`, and the push and CI fix commits SHALL read `<id>: no-mistakes: <text>`.
When no work-item id is resolved, the gate SHALL write the subject unchanged as `no-mistakes(<step>): <summary>` or `no-mistakes: <text>`.
A subject that already leads with the resolved id SHALL NOT be prefixed a second time.

#### Scenario: Step fix commit with a resolved id
- **WHEN** the resolved id is `WEB-12345` and the document step commits a fix summarized as `sync config spec`
- **THEN** the commit subject is `WEB-12345: no-mistakes(document): sync config spec`

#### Scenario: CI fix commit with a resolved id
- **WHEN** the resolved id is `WEB-12345` and the CI step commits its fixes
- **THEN** the commit subject is `WEB-12345: no-mistakes: apply CI fixes`

#### Scenario: Fix commit with no resolved id
- **WHEN** no work-item id is resolved and the lint step commits a fix summarized as `remove trailing whitespace`
- **THEN** the commit subject is `no-mistakes(lint): remove trailing whitespace`

### Requirement: Generated PR title leads with the resolved id

When a work-item id is resolved, the gate SHALL produce a PR title that leads with `<id>: ` followed by the change description, with any leading Conventional Commit type prefix removed so the title is not double-prefixed.
When no work-item id is resolved, the gate SHALL produce a Conventional Commits PR title unchanged.

#### Scenario: PR title from a ticketed author commit on an unticketed branch
- **WHEN** the resolved id is `WEB-12345` and the change description is `fix(releases): match current Jira dev-status applicationType`
- **THEN** the PR title is `WEB-12345: match current Jira dev-status applicationType`

#### Scenario: PR title with no resolved id
- **WHEN** no work-item id is resolved and the change description is `match current Jira dev-status applicationType`
- **THEN** the PR title is a Conventional Commits title such as `fix: match current Jira dev-status applicationType`
