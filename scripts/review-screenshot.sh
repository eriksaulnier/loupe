#!/usr/bin/env bash
# Renders the demo's published review on GitHub and captures it for the README as docs/assets/review.png and
# docs/assets/review-dark.png. GitHub's own render is the point, so every run posts one COMMENT review to a scratch
# pull request: a render of the Markdown anywhere else is not what a reader sees (docs/github-facts.md). Submitted
# reviews cannot be deleted, so the scratch pull request keeps every run's review; LOUPE_SCREENSHOT_REVIEW=<id>
# recaptures one already posted instead of posting again.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

scratch_repo=${LOUPE_SCREENSHOT_REPO:-eriksaulnier/loupe-format-spike}
scratch_pr=${LOUPE_SCREENSHOT_PR:-3}
playwright_version=1.56.1

for tool in gh jq perl pnpm node go; do
	command -v "$tool" >/dev/null || { echo "review-screenshot: needs $tool on PATH" >&2; exit 1; }
done

# The pills load from assets/review/v1 at a commit. main has them only once a change lands, so a branch renders at
# its own head, which MUST be pushed or every pill shows its alt text.
ref=${LOUPE_ASSETS_REF:-$(git rev-parse HEAD)}
if [[ -z ${LOUPE_ASSETS_REF:-} ]] && { ! git diff --quiet HEAD -- assets/review || [[ -n $(git ls-files --others --exclude-standard assets/review) ]]; }; then
	echo "review-screenshot: assets/review has changes not in HEAD, and the pills load from HEAD; commit and push them" >&2
	exit 1
fi
if ! out=$(gh api "repos/eriksaulnier/loupe/commits/$ref" --silent 2>&1); then
	echo "review-screenshot: cannot find $ref on github.com/eriksaulnier/loupe, which must have it pushed: $out" >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Playwright is installed into the temporary directory each run, so the repository takes on no JavaScript dependency.
# It comes before the post, so a failed setup posts nothing.
mkdir "$tmp/pw"
pnpm add --dir "$tmp/pw" "playwright@$playwright_version" --silent >/dev/null
"$tmp/pw/node_modules/.bin/playwright" install chromium >/dev/null

go run ./cmd/loupe-demo body >"$tmp/body.md"
# Every finding stays collapsed: the rows are what the picture is for, and the terminal pictures show a finding's
# contents. That also keeps it near the other README images' proportions.
REF="$ref" perl -0pi -e 's{raw\.githubusercontent\.com/eriksaulnier/loupe/main/}{raw.githubusercontent.com/eriksaulnier/loupe/$ENV{REF}/}g' "$tmp/body.md"

review=${LOUPE_SCREENSHOT_REVIEW:-}
if [[ -z $review ]]; then
	head=$(gh api "repos/$scratch_repo/pulls/$scratch_pr" --jq .head.sha)
	review=$(jq -n --rawfile body "$tmp/body.md" --arg sha "$head" '{commit_id: $sha, event: "COMMENT", body: $body}' |
		gh api "repos/$scratch_repo/pulls/$scratch_pr/reviews" --input - --jq .id)
	echo "review-screenshot: posted review $review on $scratch_repo#$scratch_pr; rerun with LOUPE_SCREENSHOT_REVIEW=$review to recapture it" >&2
fi

cat >"$tmp/shot.cjs" <<'EOF'
const { chromium } = require('playwright');
(async () => {
  const [url, review, out, scheme] = process.argv.slice(2);
  const browser = await chromium.launch();
  const page = await browser.newPage({ colorScheme: scheme, deviceScaleFactor: 2, viewport: { width: 1280, height: 900 } });
  // The whole comment card, header and border included, so the picture reads as a GitHub review and has an edge
  // against the README's own background.
  const body = page.locator(`#pullrequestreview-${review} .timeline-comment-group`).first();
  // GitHub keeps live connections open, so the page never goes network-idle, and a review posted a moment ago may
  // not be in the first render. One reload covers that.
  for (let attempt = 1; ; attempt++) {
    await page.goto(url, { waitUntil: 'load' });
    try {
      await body.waitFor({ timeout: 20000 });
      break;
    } catch (e) {
      if (attempt === 2) throw e;
    }
  }
  // The card's pointer sits 12px above it and the timeline's line runs behind it, so a capture of the card cuts
  // both off at its top edge.
  await page.addStyleTag({ content: '.timeline-comment-group::before, .timeline-comment-group::after, .TimelineItem::before { display: none !important; }' });
  // GitHub's sticky headers otherwise draw over the top of the element once it is scrolled into view. Hiding by
  // computed position does not depend on GitHub's class names.
  await page.evaluate(() => {
    for (const el of document.querySelectorAll('body *')) {
      const { position } = getComputedStyle(el);
      if (position === 'sticky' || position === 'fixed') el.style.setProperty('display', 'none', 'important');
    }
  });
  // A pill that fails to load renders as its alt text, which would ship a picture of the fallback.
  const missing = await body.evaluate(async (node) => {
    const imgs = [...node.querySelectorAll('img')];
    // An image that never settles would otherwise hang the capture; it is reported below as not loaded.
    const settled = Promise.all(imgs.map((i) => (i.complete ? 0 : new Promise((r) => { i.onload = i.onerror = r; }))));
    await Promise.race([settled, new Promise((r) => setTimeout(r, 15000))]);
    return imgs.filter((i) => i.naturalWidth === 0).map((i) => i.currentSrc || i.src);
  });
  if (missing.length > 0) throw new Error(`images did not load: ${missing.join(', ')}`);
  await body.screenshot({ path: out });
  await browser.close();
})().catch((e) => { console.error(`review-screenshot: ${e.message}`); process.exit(1); });
EOF
url="https://github.com/$scratch_repo/pull/$scratch_pr"
# Both are captured before either replaces the committed pair, so a failed run never leaves them mismatched.
NODE_PATH="$tmp/pw/node_modules" node "$tmp/shot.cjs" "$url" "$review" "$tmp/review.png" light
NODE_PATH="$tmp/pw/node_modules" node "$tmp/shot.cjs" "$url" "$review" "$tmp/review-dark.png" dark
mv "$tmp/review.png" "$tmp/review-dark.png" docs/assets/
echo "review-screenshot: wrote docs/assets/review.png and docs/assets/review-dark.png" >&2
