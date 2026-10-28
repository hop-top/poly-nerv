<?php

declare(strict_types=1);

/**
 * Dispatch a single envelope through the SDK.
 *
 * This runs in-process against the stub transport, so it needs no
 * daemon and no PECL grpc. To reach a live daemon, build the client
 * with Client::fromSocket($socket) instead (requires PECL grpc).
 *
 * Run:
 *   composer install
 *   php examples/dispatch.php
 */

require __DIR__ . '/../vendor/autoload.php';

use HopTop\Nerv\Client;
use HopTop\Nerv\EventServiceStub;
use Nerv\V1\Action;
use Nerv\V1\Decision;
use Nerv\V1\Envelope;

$socket = Client::defaultSocketPath();
fprintf(STDERR, "(would dispatch to %s once Connect-PHP ships)\n", $socket);

$allow = new Decision();
$allow->setAction(Action::ACTION_ALLOW);

$stub = (new EventServiceStub())->withQueuedDecision($allow);
$client = new Client($stub);

$envelope = new Envelope();
$envelope->setEvent('PreToolUse');
$envelope->setNervVersion('0.1.0');

$decision = $client->dispatch($envelope);

printf(
    "event=%s action=%d message=%s\n",
    $envelope->getEvent(),
    $decision->getAction(),
    $decision->getMessage(),
);

$client->close();
