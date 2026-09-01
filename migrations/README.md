# Migrations

Database-specific migrations live under `mysql`, `postgres`, and `kingbase`. Set `migration.path` to the matching directory, for example `migrations/postgres`. Review indexes, collation and online-DDL impact against production data before deployment.

Migration `000003_application_scope` intentionally adds non-null `application_id` columns without inventing a legacy application. Before applying it to a database that already contains rule data, stop writes and either map every existing rule set/version to an authoritative application in a reviewed backfill or export and recreate that data. The migration fails closed when no trustworthy mapping has been supplied.
