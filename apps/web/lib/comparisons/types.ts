import type { ReactNode } from "react";

export type ComparisonDiffRow = {
  topic: string;
  bast: string;
  competitor: string;
};

export type ComparisonFaq = {
  q: string;
  a: string;
};

export type ComparisonSection = {
  title: string;
  paragraphs: ReactNode[];
};

export type ComparisonRelated = {
  href: string;
  label: string;
};

export type ComparisonCaseStudy = {
  slug: string;
  competitorName: string;
  title: string;
  description: string;
  keywords: string[];
  lead: string;
  articleHeadline: string;
  articleDescription: string;
  diffRows: ComparisonDiffRow[];
  sections: ComparisonSection[];
  whenBetterTitle: string;
  whenBetterIntro: string;
  whenBetterItems: string[];
  whenBetterOutro: string;
  migrateTitle: string;
  migrateSteps: ReactNode[];
  faqs: ComparisonFaq[];
  related: ComparisonRelated[];
};

export const comparisonSlugs = [
  "termius",
  "putty",
  "mobaxterm",
  "securecrt",
] as const;

export type ComparisonSlug = (typeof comparisonSlugs)[number];
