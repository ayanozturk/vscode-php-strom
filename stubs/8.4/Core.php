<?php

/**
 * Interface to detect if a class is traversable using foreach.
 */
interface Traversable
{
}

/**
 * Interface for external iterators.
 *
 * @template TKey
 * @template TValue
 * @extends Traversable<TKey, TValue>
 */
interface IteratorAggregate extends Traversable
{
    /**
     * @return Traversable<TKey, TValue>
     */
    public function getIterator(): Traversable;
}

/**
 * Interface for external iterators or objects that can be iterated themselves.
 *
 * @template TKey
 * @template TValue
 * @extends Traversable<TKey, TValue>
 */
interface Iterator extends Traversable
{
    public function current(): mixed;

    public function next(): void;

    public function key(): mixed;

    public function valid(): bool;

    public function rewind(): void;
}

interface Throwable
{
    public function getMessage(): string;

    public function getCode(): int;

    public function getFile(): string;

    public function getLine(): int;

    public function getTrace(): array;

    public function getTraceAsString(): string;

    public function getPrevious(): ?Throwable;

    public function __toString(): string;
}

interface Stringable
{
    public function __toString(): string;
}

class Exception implements Throwable, Stringable
{
    public function __construct(string $message = "", int $code = 0, ?Throwable $previous = null) {}

    public function getMessage(): string { return ""; }

    public function getCode(): int { return 0; }

    public function getFile(): string { return ""; }

    public function getLine(): int { return 0; }

    public function getTrace(): array { return []; }

    public function getTraceAsString(): string { return ""; }

    public function getPrevious(): ?Throwable { return null; }

    public function __toString(): string { return ""; }
}
