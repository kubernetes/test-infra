# tiny-oncall

Tiny oncall implements sufficient oncall infrastructure for the Kubernetes test-infra oncall
rotation, and nothing else. In particular, it implements:

* Defining a rotation of n people
* Two people are selected from it once per week
* You can have some shadows, who are perma-scheduled as shadows
* Switching over at a definable time.
* Uploading the resulting file to a fixed location on GCS

It does not implement:

* Calendar integration (we can't share our calendars anyway)
* State (this burderns humans but saves substantially on complexity)
* Vacations (manually swap with someone if you have an upcoming vacation)
* Shift change notifications (but the slack-oncall-updater will tell you you're oncall if you aren't Katharine)
* Paging (ping @test-infra-oncall on slack, probably - maybe we update a mailing list in the future)

Limitations:

* Adding or removing oncallers will change who is currently oncall. Please ensure you rotate the
  list to avoid surprsing disruptions.
