<?php

/**
 * @template TKey
 * @template TValue
 * @extends Iterator<TKey, TValue>
 */
interface SeekableIterator extends Iterator
{
    public function seek(int $offset): void;
}

/**
 * @template TKey
 * @template TValue
 */
interface ArrayAccess
{
    public function offsetExists(mixed $offset): bool;

    public function offsetGet(mixed $offset): mixed;

    public function offsetSet(mixed $offset, mixed $value): void;

    public function offsetUnset(mixed $offset): void;
}

interface Countable
{
    public function count(): int;
}

interface Serializable
{
    public function serialize(): string;

    public function unserialize(string $data): void;
}

/**
 * @template TKey
 * @template TValue
 * @implements SeekableIterator<TKey, TValue>
 * @implements ArrayAccess<TKey, TValue>
 */
class ArrayIterator implements SeekableIterator, ArrayAccess, Serializable, Countable
{
    public const STD_PROP_LIST = 1;
    public const ARRAY_AS_PROPS = 2;

    public function __construct(array|object $array = [], int $flags = 0) {}

    public function append(mixed $value): void {}

    public function asort(int $flags = 0): bool { return true; }

    public function count(): int { return 0; }

    public function current(): mixed { return null; }

    public function getArrayCopy(): array { return []; }

    public function getFlags(): int { return 0; }

    public function key(): mixed { return null; }

    public function ksort(int $flags = 0): bool { return true; }

    public function natcasesort(): bool { return true; }

    public function natsort(): bool { return true; }

    public function next(): void {}

    public function offsetExists(mixed $key): bool { return true; }

    public function offsetGet(mixed $key): mixed { return null; }

    public function offsetSet(mixed $key, mixed $value): void {}

    public function offsetUnset(mixed $key): void {}

    public function rewind(): void {}

    public function seek(int $offset): void {}

    public function serialize(): string { return ""; }

    public function setFlags(int $flags): void {}

    public function uasort(callable $callback): bool { return true; }

    public function uksort(callable $callback): bool { return true; }

    public function unserialize(string $data): void {}

    public function valid(): bool { return true; }
}
