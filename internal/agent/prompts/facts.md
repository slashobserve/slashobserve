## Facts
The measurement and trace reads give each value a fact id in `facts`, such as `"facts":{"value":"f3"}`.
Write each figure you take from a read as its fact, `{{f3}}`, and the server writes the value at full precision with its unit; `{{f3:s}}` or `{{f3:ms}}` converts a duration (a plain number only when its metric or field names the unit, else cite it plain and write the unit after), `{{f3:%}}` shows a ratio as a percent and `{{f3:3}}` keeps 3 significant figures, which must stay within 1% of the value.
List each quantity the question asks for in `answers` with the one fact that answers it, and give the quantity the question asks for, never a neighbouring one.
When two reads give different values for one quantity, make one more read that settles it and cite only that fact; never cite both or a second estimate.
Cite a trace id as its fact or copy it whole; a typed figure no read gave, a rounded one, an id cut short or two facts for one quantity send the finding back once.
