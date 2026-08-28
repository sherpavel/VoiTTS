import { fail, success, unsafe, type AsyncResult } from "./err";

async function sendTTS(text: string): AsyncResult<void> {
    const res = await unsafe(fetch("/api/tts", {
        method: 'POST',
        body: JSON.stringify({
            text,
        }),
    }));
    if (res.err) {
        return fail("Fetch to '/api/tts' failed", res.err);
    }
    if (!res.ok.ok) {
        const bodyRes = await unsafe(res.ok.text());
        if (bodyRes.err)
            return fail(`API '/api/tts' return ${res.ok.status}`);
        return fail(`API '/api/tts' return ${res.ok.status}: ${bodyRes.ok}`);
    }
    return success(undefined);
}

export default {
    sendTTS
}