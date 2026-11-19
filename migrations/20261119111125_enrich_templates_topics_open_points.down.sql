-- Revert GENERAL, MOM, and TECH_REVIEW prompt templates to previous schemas

-- 1. Revert MOM Template
UPDATE templates
SET description = 'Ekstraksi agenda, daftar kehadiran, dinamika rapat, keputusan final, dan tabel action items ber-deadline.',
    prompt = 'Analisis transkrip percakapan berikut ke format Minutes of Meeting (MOM) terstruktur dengan agenda, daftar hadir, konsensus, dan action items ber-assignee dan deadline.',
    output_schema = $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "meeting_title",
    "executive_summary",
    "meeting_dynamics",
    "stakeholder_perspectives",
    "key_decisions",
    "action_items",
    "open_issues"
  ],
  "properties": {
    "meeting_title": {
      "type": "string"
    },
    "meeting_goal": {
      "type": "string"
    },
    "executive_summary": {
      "type": "string"
    },
    "meeting_dynamics": {
      "type": "object",
      "required": [
        "speaker_talk_time",
        "consensus_score",
        "sentiment_trajectory"
      ],
      "properties": {
        "speaker_talk_time": {
          "type": "array",
          "items": {
            "type": "object",
            "required": [
              "speaker",
              "percentage",
              "duration_minutes"
            ],
            "properties": {
              "speaker": {
                "type": "string"
              },
              "percentage": {
                "type": "number"
              },
              "duration_minutes": {
                "type": "number"
              }
            }
          }
        },
        "consensus_score": {
          "type": "string"
        },
        "sentiment_trajectory": {
          "type": "string"
        }
      }
    },
    "stakeholder_perspectives": {
      "type": "object",
      "required": [
        "executive_brief",
        "engineering_focus",
        "product_delivery"
      ],
      "properties": {
        "executive_brief": {
          "type": "string"
        },
        "engineering_focus": {
          "type": "string"
        },
        "product_delivery": {
          "type": "string"
        }
      }
    },
    "key_decisions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "decision",
          "root_cause_trigger",
          "contested_points",
          "quantitative_impact",
          "approved_by"
        ],
        "properties": {
          "decision": {
            "type": "string"
          },
          "root_cause_trigger": {
            "type": "string"
          },
          "contested_points": {
            "type": "string"
          },
          "quantitative_impact": {
            "type": "string"
          },
          "approved_by": {
            "type": "string"
          }
        }
      }
    },
    "action_items": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "task",
          "pic",
          "due_date",
          "priority",
          "definition_of_done",
          "cost_of_inaction"
        ],
        "properties": {
          "task": {
            "type": "string"
          },
          "pic": {
            "type": "string"
          },
          "due_date": {
            "type": "string"
          },
          "priority": {
            "type": "string",
            "enum": [
              "P0",
              "P1",
              "P2"
            ]
          },
          "definition_of_done": {
            "type": "string"
          },
          "cost_of_inaction": {
            "type": "string"
          }
        }
      }
    },
    "open_issues": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "next_meeting": {
      "type": "string"
    }
  }
}$$::jsonb,
    updated_at = CURRENT_TIMESTAMP
WHERE category_key = 'MOM';

-- 2. Revert GENERAL Template
UPDATE templates
SET description = 'Sintesis memo naratif eksekutif bergaya Cornell Notes, poin kunci kuantitatif empiris, kutipan bernilai strategis, dan agenda tindak lanjut.',
    prompt = 'Sintesis percakapan umum/diskusi eksekutif ke dalam format Executive Memo bergaya Cornell Notes, empirical takeaways, dan kutipan kunci strategis.',
    output_schema = $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "executive_overview",
    "core_themes",
    "key_takeaways",
    "notable_quotes"
  ],
  "properties": {
    "executive_overview": {
      "type": "string"
    },
    "core_themes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "theme",
          "summary_points"
        ],
        "properties": {
          "theme": {
            "type": "string"
          },
          "summary_points": {
            "type": "array",
            "items": {
              "type": "string"
            }
          }
        }
      }
    },
    "key_takeaways": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "notable_quotes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "quote",
          "speaker",
          "context"
        ],
        "properties": {
          "quote": {
            "type": "string"
          },
          "speaker": {
            "type": "string"
          },
          "context": {
            "type": "string"
          }
        }
      }
    },
    "referenced_resources": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  }
}$$::jsonb,
    updated_at = CURRENT_TIMESTAMP
WHERE category_key = 'GENERAL';

-- 3. Revert TECH_REVIEW Template
UPDATE templates
SET description = 'Rangkuman tinjauan arsitektur sistem, catatan Architectural Decision Records (ADR), mitigasi risiko reliabilitas, dan degradasi latensi.',
    prompt = 'Sintesis percakapan teknis arsitektur ke dalam format RFC/ADR dengan alternatif solusi yang ditolak, konsensus arsitektural, dan rencana mitigasi teknis.',
    output_schema = $${
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": [
    "context",
    "decisions_adopted",
    "rejected_alternatives",
    "nfr_assessment",
    "action_items"
  ],
  "properties": {
    "context": {
      "type": "string"
    },
    "decisions_adopted": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "decision",
          "technical_justification"
        ],
        "properties": {
          "decision": {
            "type": "string"
          },
          "technical_justification": {
            "type": "string"
          }
        }
      }
    },
    "rejected_alternatives": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "alternative",
          "rejection_reason"
        ],
        "properties": {
          "alternative": {
            "type": "string"
          },
          "rejection_reason": {
            "type": "string"
          }
        }
      }
    },
    "nfr_assessment": {
      "type": "object",
      "required": [
        "security",
        "performance_scalability",
        "reliability_resilience"
      ],
      "properties": {
        "security": {
          "type": "string"
        },
        "performance_scalability": {
          "type": "string"
        },
        "reliability_resilience": {
          "type": "string"
        }
      }
    },
    "technical_debt_impact": {
      "type": "string"
    },
    "action_items": {
      "type": "array",
      "items": {
        "type": "object",
        "required": [
          "task",
          "assignee",
          "target_sprint"
        ],
        "properties": {
          "task": {
            "type": "string"
          },
          "assignee": {
            "type": "string"
          },
          "target_sprint": {
            "type": "string"
          }
        }
      }
    }
  }
}$$::jsonb,
    updated_at = CURRENT_TIMESTAMP
WHERE category_key = 'TECH_REVIEW';
