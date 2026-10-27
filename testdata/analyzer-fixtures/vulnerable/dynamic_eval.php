<?php
$userInput = $_GET["expression"] ?? "";
eval($userInput);
