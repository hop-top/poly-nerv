<?php

declare(strict_types=1);

/**
 * Query a running nerv daemon for its event catalog.
 *
 * Like dispatch.php, this runs against the in-process stub. The stub
 * returns a canned descriptor list so the example output is
 * deterministic. To reach a live daemon, build the client with
 * Client::fromSocket($socket) instead (requires PECL grpc).
 *
 * Run:
 *   composer install
 *   php examples/list_events.php
 */

require __DIR__ . '/../vendor/autoload.php';

use HopTop\Nerv\Client;
use HopTop\Nerv\EventServiceStub;
use Nerv\V1\EventDescriptor;

$socket = Client::defaultSocketPath();
fprintf(STDERR, "(would list from %s once Connect-PHP ships)\n", $socket);

$pre = (new EventDescriptor())
    ->setName('PreToolUse')
    ->setCategory('tool')
    ->setOrigin('native')
    ->setBlocking(true);
$session = (new EventDescriptor())
    ->setName('SessionStart')
    ->setCategory('session')
    ->setOrigin('native');

$stub = (new EventServiceStub())->withDescriptors([$pre, $session]);
$client = new Client($stub);

foreach ($client->list() as $ev) {
    $flags = implode(' / ', [
        $ev->getBlocking() ? 'blocking' : 'non-blocking',
        $ev->getOrigin() !== '' ? $ev->getOrigin() : 'unknown',
        $ev->getCategory() !== '' ? $ev->getCategory() : 'uncategorized',
    ]);
    printf("%-28s %s\n", $ev->getName(), $flags);
}

$client->close();
