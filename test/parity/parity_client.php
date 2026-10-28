<?php

declare(strict_types=1);

/**
 * Parity client for the PHP SDK.
 *
 * Reads NERV_SOCKET from env, dials the daemon via
 * HopTop\Nerv\Client::fromSocket(), calls list() then dispatch()
 * against the canonical parity contract, and prints both markers on
 * stdout (list event name + dispatch decision message). The Go harness
 * (TestParity_PHP) asserts stdout contains both.
 *
 * Exit codes:
 *   0 — success (both markers printed)
 *   2 — environment misconfiguration (missing autoload, missing socket env)
 *   3 — PECL grpc extension not loaded (skip signal to the Go harness)
 *
 * Invoked as:
 *   NERV_SOCKET=/tmp/n.sock php test/parity/parity_client.php
 */

$autoload = __DIR__ . '/../../sdk/php/vendor/autoload.php';
if (!file_exists($autoload)) {
    fwrite(STDERR, "sdk/php/vendor missing — run: composer install --working-dir=sdk/php\n");
    exit(2);
}
require $autoload;

use HopTop\Nerv\Client;
use Nerv\V1\CLIInfo;
use Nerv\V1\Envelope;
use Nerv\V1\SessionInfo;

$socket = getenv('NERV_SOCKET');
if (!is_string($socket) || $socket === '') {
    fwrite(STDERR, "NERV_SOCKET unset\n");
    exit(2);
}

// ext-grpc is the only "skip" condition now — install via
// `pecl install grpc` (or `apt install php-grpc`). Any other failure
// (daemon down, missing decision, etc.) is a real test failure.
if (!extension_loaded('grpc')) {
    fwrite(
        STDERR,
        "PECL grpc extension not loaded — install via "
        . "`pecl install grpc` (or `apt install php-grpc`)\n"
    );
    exit(3);
}

$client = Client::fromSocket($socket);

try {
    $events = $client->list();
    $firstName = '';
    if (count($events) > 0) {
        $firstName = $events[0]->getName();
    }
    echo $firstName . "\n";

    $envelope = new Envelope();
    $envelope->setNervVersion('1.0.0-draft');
    $envelope->setEvent('PreToolUse');
    $envelope->setEventId('parity-event');

    $cli = new CLIInfo();
    $cli->setName('claude');
    $cli->setVersion('0.0.0');
    $envelope->setCli($cli);

    $session = new SessionInfo();
    $session->setId('parity-session');
    $session->setCwd('/tmp');
    $envelope->setSession($session);

    $decision = $client->dispatch($envelope);
    echo ($decision->getMessage() ?? '') . "\n";
} finally {
    $client->close();
}
