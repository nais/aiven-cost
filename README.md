# Aiven cost
this repo defines two jobs, aiven cost and kakfa cost

## Aiven cost

This job takes invoice lines from aiven and puts them into bigquery to
produce the cost of valkey, opensearch etc.

## Kafka cost
This is a job that exfiltrates invoice lines related to kafka costs
and splits them up on a per team basis and puts them into bigquery.
The calculation is, per team: `0.5*kafka_base + storage
weight*kakfa_base * 0.5 + tiered storage`. The real work is the
joining a bunch of different invoice lines from a few different aiven
api endpoints.

The views in console come from nais-billing that exposes a view that
includes the kafka_cost table defined in the code here.

run: `AIVEN_API_TOKEN=sometoken nix run`
