Overview
========

Transform is used to create relevant metrics from the data saved in the SQL
database. The SQL database contains the issues and the list of events,
but we may want to calculate additional metrics that reflect the health of
the project.  For example, we may want to understand when certain labels were
applied, and what happened to the pull-request or issues through its lifetime.

This logic is written as "Plugins". A quick look at the code
([plugins.go](plugins.go)) explains the interface of a plugin, and the type of
parameters it receives.

Plugins either wait for:
- Changes to issues (they come by order of modification)
- Events and comments: they come sorted by creation date.

Note that you will always receives changes to issues before receiving events or
comments.

The program periodically fetches from the SQL database to find changes and
pushes them to each plugin.

Walk-through: Creating a new plugin
===================================

To create a plugin, you need to implement the interface defined in
[plugin.go](plugins/plugin.go), and also register its creation there.

Let's review an example plugin, [merged.go](merged.go):

- At registration time, `NewMergedPlugin` tries to get the last measurement from
  the InfluxDB, so that it doesn't have to store data it has already written.
  Every time a plugin is run, it receives all historical events from the SQL
  Database, and it processes all of them, but only stores new ones.

- We need to implement `ReceiveIssue`, `ReceiveComment`, and `ReceiveIssueEvent`
  even if we don't use all of them.

- When we receive an Event, in this situation, we want to count how many
  "merged" events we received, so we just discard all events other than
  "merged". Then we discard events that are already in the database because they
  we have already processed them. And finally, if the event is of the correct
  type and we have not already processed it, we insert the value in the database
  through the InfluxDB interface.

- We make sure the plugin is registered in `NewCountPlugin` in [count.go](plugins/count.go) or it's
  never going to receive any events.

Testing locally
===============

In order to test this program locally, you will need a database set-up in MySQL,
populated with data. Refer to [../fetcher](../fetcher/)
documentation to see how to populate your own database.

Once it is set-up, you will also need the grafana-stack set-up locally. Refer to
[../grafana-stack](../grafana-stack/) to see how to do that.

You can then run `transform` to connect to your local instances.
