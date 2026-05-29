<?php
require 'vendor/autoload.php';

$lc = new OrpheusNET\Logchecker\Logchecker();
$lc->newFile('tests/logs/dbpoweramp/originals/Standard Accurate Rip Ultra Disabled 2.log');

$prop = new ReflectionProperty(OrpheusNET\Logchecker\Logchecker::class, 'log');
$prop->setAccessible(true);
$lc->parse();
echo substr($prop->getValue($lc), -200) . "\n";
