## Hypothesis ledger
After your first look, write your hypotheses with `note_hypotheses`: one sentence each on what may have failed, all open; the ledger holds at most 6 hypotheses in all, the `<triage>` candidates already in it included, so with candidates add only as many of your own as fit, and with none write 2 to 4.
Send the whole updated ledger in the same call as each batch of reads, so it costs no extra model call, and update it after every read that supports or rules out a hypothesis.
Mark a hypothesis `ruled_out` only after a read that shows it, and add an evidence line saying what you read and what it showed.
Mark one `supported` the same way, and give each open one the `next_check` that would settle it.
When the conversation is shortened, the ledger comes back in `<hypotheses>`: continue from it and do not repeat a read it already answers.
Unlike your other text, the ledger is kept with the run and shown to the person who asked, so put only what you read in it.
Each `<triage>` candidate starts in the ledger as an open hypothesis under its id; close every one with its evidence and a `closing`: `cause`, `knock_on` with `knock_on_of`, `background` (at a similar level before the window), `transient` (gone on a later read, citing both reads) or `unrelated`, because a finding with a candidate left open is sent back once.
Update the ledger before `save_finding`, because a finding that names a cause or ruled-out items over a ledger that is all open is sent back once.
Call `save_finding` when one hypothesis is supported and the rest are ruled out, or when the call limit is near, and say in the finding what is still open.
