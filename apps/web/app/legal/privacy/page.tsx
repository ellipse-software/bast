import Link from "next/link";

import {
  LegalList,
  LegalPage,
  LegalSection,
  legalLinkClass,
} from "@/components/legal-page";
import {
  company,
  legalUpdated,
  privacyPath,
  termsPath,
} from "@/lib/company";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = createPageMetadata({
  title: "Privacy Policy",
  description: `How ${company.legalName} trading as ${company.tradingName} collects and uses personal data for Bast.sh, including vault, telemetry, and sponsorships.`,
  path: privacyPath,
});

export default function PrivacyPage() {
  return (
    <LegalPage title="Privacy Policy" updated={legalUpdated}>
      <LegalSection id="introduction" title="1. Introduction">
        <p>
          This Privacy Policy explains how {company.legalName}, a company
          incorporated in {company.jurisdiction} with company number{" "}
          {company.companyNumber}, trading as {company.tradingName} (“we”, “us”,
          “our”), collects, uses, stores, shares, and protects personal data in
          connection with Bast.sh.
        </p>
        <p>This Policy applies to:</p>
        <LegalList>
          <li>visitors to bast.sh and related documentation;</li>
          <li>users of the Bast CLI and terminal interface;</li>
          <li>people who link a machine to Bast Vault;</li>
          <li>people who make a sponsorship payment;</li>
          <li>people who email us or otherwise contact us.</li>
        </LegalList>
        <p>
          Bast.sh is a product and hosted service of {company.legalName}. The
          CLI is open source software. Vault, this website, and related APIs are
          hosted services we operate.
        </p>
      </LegalSection>

      <LegalSection id="controller" title="2. Data controller">
        <p>
          For the UK General Data Protection Regulation (“UK GDPR”) and the Data
          Protection Act 2018, {company.legalName} is the data controller unless
          we say otherwise.
        </p>
        <p>
          Email:{" "}
          <a className={legalLinkClass} href={`mailto:${company.privacyEmail}`}>
            {company.privacyEmail}
          </a>
        </p>
        <p>
          {company.legalName}
          <br />
          {company.registeredAddress}
        </p>
      </LegalSection>

      <LegalSection id="collect" title="3. Personal data we collect">
        <h3 className="text-sm font-medium tracking-tight text-foreground">
          3.1 Website
        </h3>
        <p>
          When you visit bast.sh, our hosting and network providers may process
          standard request data such as IP address, user agent, requested URL,
          referrer, approximate country, and timestamps. We use this to operate,
          secure, and debug the site. We do not run advertising trackers or
          client-side product analytics on the public website.
        </p>

        <h3 className="text-sm font-medium tracking-tight text-foreground">
          3.2 Vault accounts
        </h3>
        <p>
          If you link a machine to Bast Vault we process the email address you
          submit, one-time sign-in codes, hashed session tokens, device
          identifiers we generate, IP address used to request codes (for rate
          limiting), and vault revision metadata (size, hash, timestamp).
        </p>
        <p>
          Vault contents (Bast-managed hosts, keys, and metadata) are encrypted
          on your machine before upload. We store an opaque ciphertext blob and
          cannot decrypt it. We do not receive your vault passphrase. Cloud
          provider credentials are not included in vault sync.
        </p>

        <h3 className="text-sm font-medium tracking-tight text-foreground">
          3.3 Sponsorships
        </h3>
        <p>
          If you sponsor Bast we process the amount, optional X username, optional
          message, and whether you asked to remain anonymous. Card and bank
          details are collected by Stripe. We do not store full payment card
          numbers.
        </p>

        <h3 className="text-sm font-medium tracking-tight text-foreground">
          3.4 Telemetry
        </h3>
        <p>
          The installer and CLI may send anonymous usage events to{" "}
          <code className="text-foreground">/api/telemetry</code>: event name,
          version, operating system, architecture, and source (installer or
          CLI). We may attach an approximate country from the request. Events
          are stored against a non-identifying distinct ID. We do not collect
          hostnames, keys, emails, project IDs, or SSH config contents in
          telemetry.
        </p>
        <p>
          Set{" "}
          <code className="text-foreground">BAST_NO_TELEMETRY=1</code> to
          disable usage telemetry and error reporting. See{" "}
          <Link href="/docs/reference/telemetry" className={legalLinkClass}>
            Telemetry
          </Link>
          .
        </p>

        <h3 className="text-sm font-medium tracking-tight text-foreground">
          3.5 Error reports
        </h3>
        <p>
          If an interactive session fails, Bast may offer to send an anonymous
          error report. That report can include the on-screen message, optional
          stack, version, OS, architecture, and a command or context label.
          Messages may contain local paths or labels. SSH session endings are
          not reported. Reports are sent only if you consent at the prompt.
        </p>

        <h3 className="text-sm font-medium tracking-tight text-foreground">
          3.6 Communications
        </h3>
        <p>
          If you contact us we process the content of that correspondence and
          any contact details you provide.
        </p>
        <p>
          We do not intentionally collect special category personal data. We do
          not knowingly collect personal data from children under 13.
        </p>
      </LegalSection>

      <LegalSection id="use" title="4. How we use personal data">
        <p>We process personal data to:</p>
        <LegalList>
          <li>provide, operate, and secure the website and Vault;</li>
          <li>send vault sign-in codes and authenticate devices;</li>
          <li>store and retrieve vault ciphertext at your request;</li>
          <li>process sponsorship payments and display public sponsors;</li>
          <li>understand product usage and diagnose failures;</li>
          <li>prevent abuse, fraud, and unauthorised access;</li>
          <li>comply with law and enforce our terms;</li>
          <li>respond to enquiries.</li>
        </LegalList>
        <p>We do not sell personal data.</p>
      </LegalSection>

      <LegalSection id="bases" title="5. Legal bases">
        <p>Under UK GDPR we rely on one or more of:</p>
        <LegalList>
          <li>
            performance of a contract (vault accounts and the hosted service);
          </li>
          <li>
            legitimate interests (security, reliability, product improvement,
            operating the website);
          </li>
          <li>consent (error reports you choose to send);</li>
          <li>legal obligation;</li>
          <li>establishment, exercise, or defence of legal claims.</li>
        </LegalList>
        <p>
          Legitimate interests include running and improving Bast.sh, keeping
          Vault available and secure, and preventing misuse. Where we rely on
          legitimate interests we consider the impact on your rights.
        </p>
      </LegalSection>

      <LegalSection id="cookies" title="6. Cookies and similar technologies">
        <p>
          Browsing bast.sh does not require an account cookie. Hosting and
          security providers may set strictly necessary cookies. Stripe may set
          cookies during sponsorship checkout. We do not use advertising
          cookies.
        </p>
        <p>
          You can control cookies in your browser. Blocking Stripe cookies may
          prevent checkout from completing.
        </p>
      </LegalSection>

      <LegalSection id="sharing" title="7. Sharing">
        <p>
          We share personal data with processors and infrastructure providers
          only as needed to run Bast.sh, including:
        </p>
        <LegalList>
          <li>hosting and content delivery (including Vercel and Cloudflare);</li>
          <li>object storage for vault ciphertext (Cloudflare R2);</li>
          <li>session, OTP, and vault metadata storage (Upstash Redis);</li>
          <li>transactional email (Cloudflare Email Sending);</li>
          <li>payments (Stripe);</li>
          <li>telemetry (PostHog);</li>
          <li>error reporting (Sentry);</li>
          <li>uptime monitoring (Better Stack);</li>
          <li>
            source hosting and release distribution (GitHub), and public profile
            data you ask us to show for sponsorships (X);
          </li>
          <li>
            professional advisers, and authorities where we are legally required
            to disclose.
          </li>
        </LegalList>
        <p>
          We may also disclose data in connection with a merger, acquisition,
          financing, or sale of assets, subject to appropriate safeguards.
        </p>
      </LegalSection>

      <LegalSection id="transfers" title="8. International transfers">
        <p>
          Personal data may be processed outside the United Kingdom, including
          by providers in the United States. Where that happens we use
          appropriate safeguards recognised under UK law, such as adequacy
          regulations, the UK International Data Transfer Agreement, or standard
          contractual clauses.
        </p>
      </LegalSection>

      <LegalSection id="retention" title="9. Retention">
        <LegalList>
          <li>Vault one-time codes: 10 minutes.</li>
          <li>Vault session tokens: 90 days, or until you log out.</li>
          <li>
            Vault ciphertext and account email mapping: until you ask us to
            delete the vault, or we close the service and give reasonable
            notice.
          </li>
          <li>
            Logging out a device revokes that session. It does not delete the
            remote vault. Email{" "}
            <a className={legalLinkClass} href={`mailto:${company.privacyEmail}`}>
              {company.privacyEmail}
            </a>{" "}
            to request deletion of vault data associated with your email.
          </li>
          <li>
            Sponsorship records: as required for accounting, tax, and dispute
            handling.
          </li>
          <li>
            Telemetry and consented error reports: only as long as useful for
            product and security analysis.
          </li>
          <li>
            Server logs: for a short operational period, longer if needed for
            security investigations.
          </li>
        </LegalList>
      </LegalSection>

      <LegalSection id="security" title="10. Security">
        <p>
          We use access controls, TLS in transit, hashed session tokens, rate
          limits on sign-in codes, and encryption of vault contents on your
          device (Argon2id and XChaCha20-Poly1305) before they reach our
          servers. No method of transmission or storage is completely secure.
        </p>
        <p>
          You are responsible for the vault passphrase, local files such as{" "}
          <code className="text-foreground">~/.config/bast/vault-passphrase</code>
          , and the security of machines that can read them. If the passphrase
          is lost we cannot recover the remote vault.
        </p>
      </LegalSection>

      <LegalSection id="rights" title="11. Your rights">
        <p>Subject to UK law, you may have the right to:</p>
        <LegalList>
          <li>access your personal data;</li>
          <li>rectify inaccurate data;</li>
          <li>erase data;</li>
          <li>restrict or object to processing;</li>
          <li>data portability;</li>
          <li>withdraw consent where we rely on consent.</li>
        </LegalList>
        <p>
          We cannot provide a plaintext export of vault contents because we
          cannot decrypt them. Requests:{" "}
          <a className={legalLinkClass} href={`mailto:${company.privacyEmail}`}>
            {company.privacyEmail}
          </a>
          . We may need to verify your identity. We may refuse or limit requests
          where the law allows.
        </p>
      </LegalSection>

      <LegalSection id="third-parties" title="12. Third-party services">
        <p>
          Cloud sync with AWS, Google Cloud, Microsoft Azure, and other
          providers runs on your machine using credentials you supply locally.
          Those providers process data under their own terms and privacy
          notices. We are not responsible for third-party sites or services we
          do not operate.
        </p>
        <p>
          If you self-host the web app, this Policy does not apply to that
          deployment. You are the controller for any personal data your instance
          processes.
        </p>
      </LegalSection>

      <LegalSection id="changes" title="13. Changes">
        <p>
          We may update this Policy. The current version is published at{" "}
          <Link href={privacyPath} className={legalLinkClass}>
            bast.sh{privacyPath}
          </Link>
          . Material changes may be notified by email or in the product where
          appropriate.
        </p>
      </LegalSection>

      <LegalSection id="complaints" title="14. Complaints">
        <p>
          Contact us first if you have a concern. You may also lodge a complaint
          with the Information Commissioner’s Office:{" "}
          <a
            className={legalLinkClass}
            href="https://ico.org.uk"
            target="_blank"
            rel="noopener noreferrer"
          >
            ico.org.uk
          </a>
          .
        </p>
      </LegalSection>

      <LegalSection id="contact" title="15. Contact">
        <p>
          Privacy:{" "}
          <a className={legalLinkClass} href={`mailto:${company.privacyEmail}`}>
            {company.privacyEmail}
          </a>
        </p>
        <p>
          Terms:{" "}
          <Link href={termsPath} className={legalLinkClass}>
            Terms of Service
          </Link>
        </p>
        <p>
          {company.legalName}
          <br />
          {company.registeredAddress}
        </p>
      </LegalSection>
    </LegalPage>
  );
}
