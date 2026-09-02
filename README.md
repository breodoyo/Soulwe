# Soulwe

*Your soul. Your way. Your safe space.*

Soulwe is a **culturally aware mental health companion for East Africa**.

It provides anonymous peer support, AI assisted journaling, therapist matching, and breathing exercises designed around African cultures, languages, and the stigma that often makes it difficult to ask for help.

## The Problem

Mental health support in East Africa is difficult to access for many people. The problem is not only a lack of services. Several barriers prevent people from seeking help.

### 1. Stigma

Many people fear being judged or misunderstood when they talk about their mental health. Struggling can be seen as weakness, a family problem, or even a spiritual failure.

### 2. Cost

Professional therapy can be expensive, especially for young people, students, and people with limited income.

### 3. Cultural Gap

Many mental health apps are designed for Western audiences. They do not always understand experiences such as family pressure, being the firstborn, community expectations, grief, or the pressure to "be strong."

### 4. Lack of Privacy

Some people are not ready to tell their family, friends, religious leaders, or even a therapist that they are struggling. They need a safe place to start anonymously.

### 5. Limited Community Support

African communities have a strong culture of sharing problems and supporting one another. Soulwe brings this idea into a safe, structured, and anonymous digital space.

## How Soulwe Helps

| Problem                  | Soulwe's Approach                                         |
| ------------------------ | --------------------------------------------------------- |
| Stigma                   | Anonymous identity by default                             |
| High therapy costs       | Free mental wellness tools and affordable therapy options |
| Cultural mismatch        | Content designed around East African experiences          |
| Fear of being identified | No real name required for peer support                    |
| Lack of safe community   | Anonymous peer support circles                            |

## Who Soulwe Is For

Soulwe is for people across East Africa who are dealing with:

* Anxiety and stress
* Grief and loss
* Family pressure
* Work or school burnout
* Loneliness
* Young adult challenges
* Difficult emotions
* The need for a private place to process their feelings

## Core Features

### Anonymous Identity

Users can participate in peer support circles using an anonymous identity such as **"Anon Baobab."**

### AI Assisted Journaling

Users can privately write about their thoughts and experiences and receive supportive AI generated reflections.

> The AI companion is not a therapist and does not replace professional mental health care.

### Peer Support Circles

Anonymous topic based communities where people can connect around experiences such as:

* Grief
* Family pressure
* Work burnout
* Relationships
* Young adult identity

### Therapist Matching

Users can find therapists based on factors such as:

* Language
* Specialty
* Price
* Availability

### Breathing Exercises

Guided breathing exercises such as **4 7 8 breathing** and **box breathing** help users practice simple relaxation techniques.

## Guiding Principles

### Privacy First

No selling personal data. No advertising. Anonymous by default.

### Culturally Grounded

Soulwe is designed around East African experiences rather than simply adapting Western mental health products.

### Accessible

Core mental wellness features should remain accessible to people regardless of income.

### Human Support Matters

Soulwe does not try to replace therapists or mental health professionals. It provides an entry point and helps users find professional support when needed.

## Tech Stack

| Layer          | Technology                  | Purpose             |
| -------------- | --------------------------- | ------------------- |
| Backend        | Go + Chi                    | REST API            |
| Database       | PostgreSQL                  | Data storage        |
| Authentication | JWT                         | User authentication |
| Frontend       | React + TypeScript + Vite   | Web application     |
| Styling        | CSS Modules + Design Tokens | UI styling          |
| AI             | Anthropic Claude API        | Journal reflections |
| Deployment     | Render + Vercel             | Hosting             |

## Project Structure

```text
soulwe/
├── README.md
├── docs/
│   ├── ARCHITECTURE.md
│   ├── API.md
│   ├── DATABASE.md
│   ├── DEVELOPMENT.md
│   └── DEPLOYMENT.md
│
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── auth/
│   │   ├── journal/
│   │   ├── circle/
│   │   ├── therapist/
│   │   ├── breathing/
│   │   └── user/
│   ├── db/
│   │   ├── migrations/
│   │   └── queries/
│   ├── middleware/
│   └── config/
│
└── frontend/
    └── src/
        ├── pages/
        ├── components/
        ├── hooks/
        ├── lib/
        ├── styles/
        └── types/
```

## Vision

Soulwe is not trying to replace psychiatry or professional therapy.

It aims to **close the gap between nothing and something**.

For someone who is not ready to see a therapist, tell their family, or talk openly about their struggles, Soulwe provides a safe place to start.

From anonymous support and journaling to professional help, Soulwe aims to make mental health support **more private, accessible, affordable, and culturally relevant for East Africa.**

## Status

Soulwe is currently under development.

More features, documentation, and integrations are being added as development continues.

## Getting Started

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for instructions on running Soulwe locally.

## Links

* **Production:** https://soulwe.vercel.app *(Coming soon)*
* **API:** https://soulwe-api.onrender.com *(Coming soon)*
* **GitHub:** https://github.com/breodoyo/soulwe
