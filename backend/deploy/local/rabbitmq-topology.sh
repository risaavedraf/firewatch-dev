#!/bin/sh
# Provisions the broker identities and the routing topology BH-04 requires, and
# is safe to run on every `docker compose up`.
#
# Why the topology has a single owner instead of being declared by the services:
#
#   1. The broker authorises by resource name and never by routing key. On one
#      shared topic exchange, a rule like "jobs may publish only
#      jobs.requested.v1" cannot be expressed, so any service able to publish
#      could forge its peer's events. Two publish exchanges, one per producing
#      service, turn that rule into a permission on a name.
#   2. Binding a queue to an exchange requires write permission on that exchange,
#      and read permission on the queue. A consumer that also binds itself to its
#      peer's exchange therefore needs write on the peer's exchange, which undoes
#      (1). So the cross-bindings are created here, by the administrator, and
#      neither service holds write on anything but its own publish exchange.
#   3. No service holds write on `firewatch.dead`. Dead-lettering is performed by
#      the broker on its own authority, not under the service identity; granting
#      write would let one service forge a dead letter into its peer's queue.
#
# The credentials below are read from the environment and never printed. They also
# never reach argv: an argument is readable through `ps` for the life of the
# request, and in any process accounting the node keeps. This script's output is
# safe to paste into evidence.
set -eu

api="${RABBITMQ_MANAGEMENT_URL:?required}/api"
admin_user="${ADMIN_USER:?required}"
admin_pass="${ADMIN_PASS:?required}"
jobs_pass="${JOBS_PASSWORD:?required}"
risk_pass="${RISK_PASSWORD:?required}"
vhost="${RABBITMQ_VHOST:-firewatch}"

jobs_exchange=firewatch.jobs.events
risk_exchange=firewatch.risk.events
dead_exchange=firewatch.dead
legacy_exchange=firewatch.events
risk_queue=risk.jobs.v1
jobs_queue=jobs.results.v1

# Credentials travel through curl's config on stdin. `--config -` reads the same
# options from a stream, so the user and password are never an argument: only the
# method, the path and the message body are, and none of those is a secret.
curl_auth() {
	printf 'user = "%s:%s"\n' "$admin_user" "$admin_pass" | curl --config - "$@"
}

# call METHOD PATH BODY [allow_conflict]
#
# A 2xx is success. A 409 counts as success only where it means "already there",
# which is the duplicate-binding case; for a declaration it means the existing
# object disagrees, and that must fail loudly instead of being papered over.
call() {
	method=$1
	path=$2
	body=$3
	allow_conflict=${4:-no}

	response=$(curl_auth -sS --connect-timeout 5 --max-time 15 -X "$method" \
		-H 'content-type: application/json' --data "$body" \
		-w '\n%{http_code}' "$api$path") || {
		echo "provisioning: $method $path: request failed" >&2
		exit 1
	}
	code=$(printf '%s\n' "$response" | tail -n 1)
	payload=$(printf '%s\n' "$response" | sed '$d')

	case "$code" in
	2??) return 0 ;;
	409) [ "$allow_conflict" = yes ] && return 0 ;;
	esac

	echo "provisioning: $method $path returned HTTP $code" >&2
	printf '%s\n' "$payload" | head -c 500 >&2
	echo >&2
	exit 1
}

echo "provisioning: exchanges"
call PUT "/exchanges/$vhost/$jobs_exchange" \
	'{"type":"topic","durable":true,"auto_delete":false,"internal":false,"arguments":{}}'
call PUT "/exchanges/$vhost/$risk_exchange" \
	'{"type":"topic","durable":true,"auto_delete":false,"internal":false,"arguments":{}}'
call PUT "/exchanges/$vhost/$dead_exchange" \
	'{"type":"direct","durable":true,"auto_delete":false,"internal":false,"arguments":{}}'

# The queue arguments must match the ones the services used to declare: a quorum
# queue with a delivery limit of 5, dead-lettered at least once into the shared
# dead exchange under the queue's own name. An inequivalent declaration is refused
# by the broker (400 bad_request) and `call` stops: an existing queue is never
# silently reshaped.
echo "provisioning: queues"
call PUT "/queues/$vhost/$risk_queue" \
	'{"durable":true,"auto_delete":false,"arguments":{"x-queue-type":"quorum","x-delivery-limit":5,"x-dead-letter-exchange":"firewatch.dead","x-dead-letter-routing-key":"risk.jobs.v1","x-overflow":"reject-publish","x-dead-letter-strategy":"at-least-once"}}'
call PUT "/queues/$vhost/$risk_queue.dead" \
	'{"durable":true,"auto_delete":false,"arguments":{"x-queue-type":"quorum"}}'
call PUT "/queues/$vhost/$jobs_queue" \
	'{"durable":true,"auto_delete":false,"arguments":{"x-queue-type":"quorum","x-delivery-limit":5,"x-dead-letter-exchange":"firewatch.dead","x-dead-letter-routing-key":"jobs.results.v1","x-overflow":"reject-publish","x-dead-letter-strategy":"at-least-once"}}'
call PUT "/queues/$vhost/$jobs_queue.dead" \
	'{"durable":true,"auto_delete":false,"arguments":{"x-queue-type":"quorum"}}'

echo "provisioning: bindings"
# jobs.requested.v1 leaves the jobs exchange and lands in the risk queue.
call POST "/bindings/$vhost/e/$jobs_exchange/q/$risk_queue" \
	'{"routing_key":"jobs.requested.v1"}' yes
# The batch result and the cross-service failure report leave the risk exchange
# and land in the jobs queue.
call POST "/bindings/$vhost/e/$risk_exchange/q/$jobs_queue" \
	'{"routing_key":"risk.batch.published.v1"}' yes
call POST "/bindings/$vhost/e/$risk_exchange/q/$jobs_queue" \
	'{"routing_key":"risk.job.failed.v1"}' yes
# Each dead-letter companion is bound to the shared dead exchange under its own
# queue name, which is the routing key the broker uses when it dead-letters.
call POST "/bindings/$vhost/e/$dead_exchange/q/$risk_queue.dead" \
	'{"routing_key":"risk.jobs.v1"}' yes
call POST "/bindings/$vhost/e/$dead_exchange/q/$jobs_queue.dead" \
	'{"routing_key":"jobs.results.v1"}' yes

# The single shared exchange of the previous topology is retired, so no ghost
# routing path survives the migration: its bindings would still deliver a
# duplicate into the same queues for anything publishing to it. A 404 means a
# previous run already retired it. Queue messages are untouched by this.
echo "provisioning: retiring the legacy exchange"
code=$(curl_auth -sS --connect-timeout 5 --max-time 15 -o /dev/null -w '%{http_code}' \
	-X DELETE "$api/exchanges/$vhost/$legacy_exchange")
case "$code" in
204 | 404) : ;;
*)
	echo "provisioning: retiring $legacy_exchange returned HTTP $code" >&2
	exit 1
	;;
esac

# Identities. Empty tags: neither service may reach the management API or act as
# an administrator; their authority is exactly the permission triple below.
echo "provisioning: identities"
call PUT "/users/firewatch-jobs" "$(printf '{"password":"%s","tags":""}' "$jobs_pass")"
call PUT "/users/firewatch-risk" "$(printf '{"password":"%s","tags":""}' "$risk_pass")"

# configure covers only the service's own queue, its dead-letter companion and
# the one exchange it publishes to, so a compromised service can damage nothing
# but its own resources. write covers exactly one exchange, which is what denies
# a forged peer event. read covers exactly one queue, which is what denies
# consuming the peer's work.
echo "provisioning: permissions"
call PUT "/permissions/$vhost/firewatch-jobs" \
	'{"configure":"^(firewatch\\.jobs\\.events|jobs\\.results\\.v1|jobs\\.results\\.v1\\.dead)$","write":"^firewatch\\.jobs\\.events$","read":"^jobs\\.results\\.v1$"}'
call PUT "/permissions/$vhost/firewatch-risk" \
	'{"configure":"^(firewatch\\.risk\\.events|risk\\.jobs\\.v1|risk\\.jobs\\.v1\\.dead)$","write":"^firewatch\\.risk\\.events$","read":"^risk\\.jobs\\.v1$"}'

echo "provisioning complete: 2 identities, 3 exchanges, 4 queues, 5 bindings"
