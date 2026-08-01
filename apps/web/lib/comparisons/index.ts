import { comparisonSlugs } from "@/lib/comparisons/types";

export { comparisonSlugs };

export const comparisonPaths = comparisonSlugs.map((slug) => `/${slug}`);
