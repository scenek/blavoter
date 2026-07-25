export function formatVoteCount(count) {
    if (count === 0) return "0 hlasů";
    if (count === 1) return "1 hlas";
    return `${count} hlasy`;
}
