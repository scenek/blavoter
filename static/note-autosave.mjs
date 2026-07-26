export function createNoteAutosave({
    initialNote = "",
    saveNote,
    onState = () => {},
    delayMs = 700,
    setTimer = globalThis.setTimeout,
    clearTimer = globalThis.clearTimeout,
}) {
    if (typeof saveNote !== "function") {
        throw new TypeError("saveNote must be a function");
    }

    let draft = String(initialNote);
    let confirmed = String(initialNote);
    let revision = 0;
    let timer;
    let inFlight = null;
    let queued = false;

    const emit = (state, error) => {
        onState(state, { draft, confirmed, error });
    };

    const cancelTimer = () => {
        if (timer === undefined) return;
        clearTimer(timer);
        timer = undefined;
    };

    const schedule = () => {
        cancelTimer();
        timer = setTimer(() => {
            timer = undefined;
            return flush();
        }, delayMs);
    };

    const setDraft = value => {
        draft = String(value);
        revision++;
        if (inFlight) {
            queued = true;
        } else {
            schedule();
        }
        emit("idle");
    };

    const flush = async () => {
        cancelTimer();
        if (inFlight) {
            queued = true;
            await inFlight;
            return confirmed;
        }

        const value = draft.trim();
        if (value === confirmed) {
            emit("idle");
            return confirmed;
        }

        const requestRevision = revision;
        inFlight = (async () => {
            emit("saving");
            try {
                const response = await saveNote(value);
                const canonical = typeof response === "string" ? response : value;
                confirmed = canonical;
                if (revision === requestRevision) {
                    draft = canonical;
                    emit("saved");
                } else {
                    queued = true;
                }
            } catch (error) {
                if (revision === requestRevision) {
                    emit("error", error);
                } else {
                    queued = true;
                }
            } finally {
                inFlight = null;
                if (queued) {
                    queued = false;
                    await flush();
                }
            }
            return confirmed;
        })();

        return inFlight;
    };

    return {
        setDraft,
        flush,
        getDraft: () => draft,
        getConfirmed: () => confirmed,
    };
}
