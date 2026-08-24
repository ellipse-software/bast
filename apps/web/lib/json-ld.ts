import { company } from "@/lib/company";
import { defaultDescription, siteName } from "@/lib/metadata";
import { openApiUrl, siteUrl, skillUrl } from "@/lib/site";

export const organizationId = `${company.website}/#organization`;
export const softwareId = `${siteUrl}/#software`;
export const websiteId = `${siteUrl}/#website`;

export function organizationJsonLd() {
  return {
    "@type": "Organization",
    "@id": organizationId,
    name: company.tradingName,
    legalName: company.legalName,
    url: company.website,
    email: company.legalEmail,
    identifier: company.companyNumber,
    address: {
      "@type": "PostalAddress",
      streetAddress: company.address.streetAddress,
      addressLocality: company.address.addressLocality,
      postalCode: company.address.postalCode,
      addressCountry: company.address.addressCountry,
    },
    contactPoint: [
      {
        "@type": "ContactPoint",
        contactType: "customer support",
        email: company.legalEmail,
        url: `${siteUrl}/contact`,
        availableLanguage: "English",
      },
      {
        "@type": "ContactPoint",
        contactType: "privacy",
        email: company.privacyEmail,
        url: `${siteUrl}/privacy`,
        availableLanguage: "English",
      },
    ],
    sameAs: [...company.sameAs],
  };
}

export function softwareApplicationJsonLd() {
  return {
    "@type": "SoftwareApplication",
    "@id": softwareId,
    name: siteName,
    alternateName: "Bast",
    url: siteUrl,
    description: defaultDescription,
    applicationCategory: "DeveloperApplication",
    operatingSystem: "macOS, Linux, Windows 11",
    softwareRequirements: "OpenSSH (ssh, ssh-keygen, ssh-add)",
    license: "https://opensource.org/licenses/MIT",
    isAccessibleForFree: true,
    downloadUrl: "https://github.com/ellipse-software/bast/releases",
    installUrl: `${siteUrl}/cli`,
    featureList: [
      "SSH host picker for ~/.ssh/config",
      "OpenSSH key manager",
      "SFTP dual-pane file browser",
      "Encrypted vault sync",
      "Cloud VM import from GCP, AWS, Azure, DigitalOcean, box.ascii.dev, and Upstash Box",
      "JSON CLI for scripts and agents",
    ],
    offers: {
      "@type": "Offer",
      price: "0",
      priceCurrency: "USD",
    },
    author: { "@id": organizationId },
    publisher: { "@id": organizationId },
  };
}

export function websiteJsonLd() {
  return {
    "@type": "WebSite",
    "@id": websiteId,
    name: siteName,
    url: siteUrl,
    description: defaultDescription,
    publisher: { "@id": organizationId },
    inLanguage: "en",
    hasPart: [
      { "@type": "WebPage", name: "Bast.sh OpenAPI", url: openApiUrl },
      { "@type": "WebPage", name: "Bast.sh agent docs", url: `${siteUrl}/llms.txt` },
      { "@type": "WebPage", name: "Bast.sh agent skill", url: skillUrl },
    ],
  };
}

export function identityGraphJsonLd() {
  return {
    "@context": "https://schema.org",
    "@graph": [
      organizationJsonLd(),
      softwareApplicationJsonLd(),
      websiteJsonLd(),
    ],
  };
}
