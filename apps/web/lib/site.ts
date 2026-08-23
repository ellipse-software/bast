export const siteUrl = "https://bast.sh";

export const llmsTxtUrl = `${siteUrl}/llms.txt`;
export const llmsFullUrl = `${siteUrl}/llms-full.txt`;
export const skillUrl = `${siteUrl}/bast.skill.md`;
export const installSkillUrl = `${siteUrl}/install-skill.sh`;
export const openApiUrl = `${siteUrl}/openapi.json`;
export const sitemapUrl = `${siteUrl}/sitemap.xml`;
export const skillsRepo = "ellipse-software/bast";

/** Preferred install via Vercel Skills CLI (npx skills). */
export const installSkillCommand = `npx skills add ${skillsRepo} -g -y`;

/** Project-local install (commit agent skill dirs with the repo). */
export const installSkillProjectCommand = `npx skills add ${skillsRepo} -y`;

/** Curl fallback when Node/npx is unavailable. */
export const installSkillCurlCommand = `curl -fsSL ${installSkillUrl} | sh`;
