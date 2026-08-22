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
  trademarkNotice,
} from "@/lib/company";
import { bastRepoUrl } from "@/lib/github";
import { createPageMetadata } from "@/lib/metadata";

export const metadata = createPageMetadata({
  title: "Terms of Service",
  description: `Terms of Service for Bast.sh, a product and hosted service of ${company.legalName} trading as ${company.tradingName}.`,
  path: termsPath,
});

export default function TermsPage() {
  return (
    <LegalPage title="Terms of Service" updated={legalUpdated}>
      <LegalSection id="about" title="1. About us">
        <p>
          These Terms of Service (“Terms”) govern your use of Bast.sh, including
          the website, documentation, installer, CLI and terminal application,
          Bast Vault, and related APIs (together, the “Services”).
        </p>
        <p>
          The Services are provided by {company.legalName}, a company
          incorporated in {company.jurisdiction} with company number{" "}
          {company.companyNumber}, whose registered office is{" "}
          {company.registeredAddress}, trading as {company.tradingName} (“we”,
          “us”, “our”). Bast.sh is a product and hosted service of{" "}
          {company.legalName}.
        </p>
        <p>
          By using the Services you agree to these Terms. If you use the
          Services for an organisation, you represent that you have authority to
          bind that organisation.
        </p>
      </LegalSection>

      <LegalSection id="software" title="2. The software">
        <p>
          The Bast CLI and terminal application are made available under the MIT
          Licence published in the{" "}
          <a
            className={legalLinkClass}
            href={bastRepoUrl}
            target="_blank"
            rel="noopener noreferrer"
          >
            public repository
          </a>
          . Nothing in these Terms limits rights granted to you under that
          licence for the software itself.
        </p>
        <p>
          These Terms govern use of our hosted Services (including bast.sh,
          Vault, sign-in, telemetry ingest, and sponsorship checkout) and your
          conduct when using Bast with those Services.
        </p>
      </LegalSection>

      <LegalSection id="vault" title="3. Vault and accounts">
        <p>
          Vault is an optional hosted sync service. You authenticate with an
          email address and a one-time code. You choose a passphrase on your
          machine. Hosts, keys, and metadata that Vault syncs are encrypted on
          your device before upload. We store ciphertext and revision metadata.
          We cannot decrypt your vault or reset a lost passphrase so that
          existing ciphertext becomes readable.
        </p>
        <p>You must:</p>
        <LegalList>
          <li>keep sign-in codes, session tokens, and passphrases confidential;</li>
          <li>use an email address you control;</li>
          <li>
            understand that a force passphrase reset overwrites the remote vault
            with this machine’s managed state and discards remote-only data;
          </li>
          <li>
            keep local copies and backups you need; we are not your backup
            provider.
          </li>
        </LegalList>
        <p>
          You may point Bast at a self-hosted API. We are not responsible for
          self-hosted deployments.
        </p>
      </LegalSection>

      <LegalSection id="acceptable-use" title="4. Acceptable use">
        <p>You shall not, and shall not permit anyone else to:</p>
        <LegalList>
          <li>use the Services unlawfully;</li>
          <li>
            probe, disrupt, or overload Vault, the website, or related
            infrastructure;
          </li>
          <li>bypass rate limits, authentication, or size limits;</li>
          <li>
            upload malware or content you do not have the right to store or
            sync;
          </li>
          <li>
            use the Services to attack, scan, or access systems without
            authorisation;
          </li>
          <li>
            resell hosted Vault as a service to third parties without our
            written agreement.
          </li>
        </LegalList>
        <p>
          You are responsible for how you use Bast to reach your own servers and
          for complying with the terms of AWS, Google Cloud, Microsoft Azure,
          and any other provider you connect.
        </p>
      </LegalSection>

      <LegalSection id="sponsorships" title="5. Sponsorships">
        <p>
          Sponsorship payments on bast.sh are voluntary contributions to support
          development. Unless we state otherwise at checkout, they do not buy a
          subscription, extra Vault capacity, an SLA, or a licence beyond the
          MIT Licence.
        </p>
        <p>
          Payments are processed by Stripe. Amounts are in US dollars. The
          charge may appear as Bast or {company.legalName}. We do not store full
          card numbers. Sponsorships are non-refundable except where law
          requires otherwise.
        </p>
      </LegalSection>

      <LegalSection id="ip" title="6. Intellectual property">
        <p>
          We and our licensors own the Bast.sh name, site, documentation, and
          hosted service (other than your vault contents and materials you
          submit). The CLI remains available under the MIT Licence.
        </p>
        <p>
          You retain ownership of data you encrypt into Vault and of content you
          submit as a sponsorship message. You grant us a licence to store and
          transmit vault ciphertext to provide the service, and to display a
          public sponsor name, handle, amount, and message if you do not opt to
          remain anonymous.
        </p>
      </LegalSection>

      <LegalSection id="third-parties" title="7. Third-party services and marks">
        <p>{trademarkNotice}</p>
        <p>
          The Services may depend on third-party infrastructure, APIs, and
          software. Your use of those providers is subject to their terms. We
          are not responsible for failures, changes, or security incidents at
          providers outside our reasonable control.
        </p>
      </LegalSection>

      <LegalSection id="privacy" title="8. Privacy">
        <p>
          Our{" "}
          <Link href={privacyPath} className={legalLinkClass}>
            Privacy Policy
          </Link>{" "}
          explains how we process personal data. Each party must comply with
          applicable data protection law. You are responsible for the lawfulness
          of data you choose to sync, including any personal data in host labels
          or notes.
        </p>
      </LegalSection>

      <LegalSection id="availability" title="9. Availability and changes">
        <p>
          Hosted Services are provided without a service level commitment unless
          we agree one in writing. We may modify, suspend, or discontinue Vault
          or any part of the Services. Where practicable we will give notice of
          material withdrawals.
        </p>
        <p>
          Preview, nightly, and experimental builds may be incomplete or
          withdrawn at any time.
        </p>
      </LegalSection>

      <LegalSection id="warranties" title="10. Warranties">
        <p>
          The software is provided under the MIT Licence “as is”. For hosted
          Services we will use reasonable skill and care. Except as required by
          law, we do not warrant that the Services will be uninterrupted,
          secure, or error-free, or that vault ciphertext can be recovered
          without your passphrase.
        </p>
        <p>
          Nothing in these Terms excludes statutory rights that cannot be
          excluded, including rights of consumers under UK law.
        </p>
      </LegalSection>

      <LegalSection id="liability" title="11. Liability">
        <p>
          Nothing in these Terms limits or excludes liability for death or
          personal injury caused by negligence, fraud, or any liability that
          cannot legally be limited.
        </p>
        <p>
          Subject to that, we are not liable for loss of profit, revenue,
          business, goodwill, opportunity, or data, or for indirect or
          consequential loss. Our total aggregate liability arising out of the
          hosted Services is limited to the greater of £100 and the sponsorship
          amounts you paid us in the 12 months before the claim.
        </p>
        <p>
          You are responsible for SSH access, key handling, and the systems you
          connect to. Unauthorised access to machines is your responsibility,
          not ours.
        </p>
      </LegalSection>

      <LegalSection id="suspension" title="12. Suspension and termination">
        <p>
          We may suspend or terminate hosted access if we reasonably believe
          there is a security threat, abuse, legal risk, or a material breach of
          these Terms. You may stop using Vault by logging out. Logging out does
          not delete remote ciphertext; request deletion as described in the
          Privacy Policy.
        </p>
        <p>
          Clauses that by their nature should survive (including intellectual
          property, liability, and governing law) remain in force.
        </p>
      </LegalSection>

      <LegalSection id="law" title="13. Governing law">
        <p>
          These Terms and any dispute or claim arising out of them are governed
          by the laws of England and Wales. The courts of England and Wales
          have exclusive jurisdiction, except that consumers may also bring
          proceedings in the courts of their UK country of residence where
          required by law.
        </p>
      </LegalSection>

      <LegalSection id="general" title="14. General">
        <p>
          If a provision is unenforceable, the rest remains in force. We may
          update these Terms by publishing a new version at{" "}
          <Link href={termsPath} className={legalLinkClass}>
            bast.sh{termsPath}
          </Link>
          . Continued use of the hosted Services after the effective date
          constitutes acceptance of the updated Terms where the law allows.
        </p>
        <p>
          These Terms do not create a partnership, agency, or employment
          relationship. A person who is not a party has no rights under the
          Contracts (Rights of Third Parties) Act 1999.
        </p>
      </LegalSection>

      <LegalSection id="contact" title="15. Contact">
        <p>
          Legal notices:{" "}
          <a className={legalLinkClass} href={`mailto:${company.legalEmail}`}>
            {company.legalEmail}
          </a>
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
