import type { ReactNode } from "react";

export type GuideSection = {
  title: string;
  paragraphs: ReactNode[];
};

export type GuidePage = {
  slug: string;
  title: string;
  description: string;
  keywords: string[];
  lead: string;
  problemTitle: string;
  problem: ReactNode[];
  solutionTitle: string;
  solution: ReactNode[];
  stepsTitle: string;
  steps: ReactNode[];
  sections?: GuideSection[];
};
