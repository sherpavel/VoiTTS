export type Err = {
    message: string;
    next?: Err;
    toString(): string;
}
export const Err = (message: string, next?: Err): Err => ({
    message,
    next,
    toString() {
        if (next) return message + ": " + next.toString();
        return message;
    }
});

export type Result<T> =
    {
        ok: T;
        err?: undefined;
    } | {
        err: Err;
    }
export type AsyncResult<T> = Promise<Result<T>>;

export type Unwrap<R> =
    R extends Promise<infer U>
    ? U extends { ok: infer V } ? V : never
    : R extends { ok: infer V } ? V : never;

export function unsafe<T>(promise: Promise<T>, err?: Err): AsyncResult<T>;
export function unsafe<T>(func: () => T, err?: Err): Result<T>;
export function unsafe<T>(
    promiseOrFunc: Promise<T> | (() => T),
    err?: Err,
): Promise<Result<T>> | Result<T> {
    if (promiseOrFunc instanceof Promise) {
        return safeAsync(promiseOrFunc, err);
    }
    return safeSync(promiseOrFunc, err);
}

async function safeAsync<T>(
    promise: Promise<T>,
    err?: Err
): Promise<Result<T>> {
    try {
        const data = await promise;
        return { ok: data };
    } catch (e) {
        if (err !== undefined) {
            return { err: err };
        }
        if (e instanceof Error) {
            return { err: Err(e.message) };
        }
        return { err: Err("Something went wrong") };
    }
}

function safeSync<T>(
    func: () => T,
    err?: Err
): Result<T> {
    try {
        const data = func();
        return { ok: data };
    } catch (e) {
        if (err !== undefined) {
            return { err: err };
        }
        if (e instanceof Error) {
            return { err: Err(e.message) };
        }
        return { err: Err("Something went wrong") };
    }
}

export function success<T>(result: T): Result<T> {
    return {
        ok: result
    }
}
export function fail<T>(message: string, next?: Err): Result<T> {
    return {
        err: Err(message, next)
    }
}

export async function wrapError<T>(result: AsyncResult<T>, errText: string, onerror?: (err: Err) => void): AsyncResult<T> {
    const res = await result;
    if (res.err) {
        onerror?.(res.err);
        return fail(errText);
    }
    return res;
}
