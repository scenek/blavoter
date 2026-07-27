export function formatVoteCount(count) {
    if (count === 0) return "0 hlasů";
    if (count === 1) return "1 hlas";
    if (count === 2) return "2 hlasy";
    if (count === 3) return "3 hlasy";
    if (count === 4) return "4 hlasy";
    return `${count} hlasů`;
}
