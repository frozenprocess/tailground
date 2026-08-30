Tailground is a safe place to try out Tailscale policy syntax and check or validate its impact on your Tailnet.

### The big picture

Securing a network is difficult and it has a massive learning curve since an intentional syntax mistake can cause a huge problem. Tailground offers an in browser evaluation engine that validates your policy syntax, applies it to the scenario and gives you the result. 

> [!why] I strongly believe Humans are visual learners
> and we need a strong mental model to understand something.
> On top of that policy mistakes are usually difficult to spot due to the complexity of some cases. 
> Tailground is an answer for these problems and as its core it tries to solve one simple problem, can device A reach device B if I write this policy.

## Get to know the Tailground

| Where | What it is |
| --- | --- |
| **Left panel** | **Tailnet** tab (the mock network) and **Learn** tab (these lessons) |
| **Right, top** | **Policy** — the editor, here is where you make and break stuff safely. |
| **Right, bottom** | **Matrix** and **Graph** — the results |

[Show me the three areas](show:layout)

Keep in mind that at any time you can adjust the layout by readjusting the dividers.

### 1. The Policy editor

[Show me the Policy editor](show:editor)

This is the file you would paste into the Tailscale admin console, written in
HuJSON, which is JSON that allows `//` comments and trailing commas.

- **Autocomplete.** Type a key such as `grants` or `ssh` and accept the
  suggestion — it inserts a ready-made rule to fill in. Inside `src`, `dst` and
  `target` it offers `*`, `tag:`, `group:`, the autogroups and `svc:`; inside
  `action` it offers the valid actions.
- **Hover** over any key to see what it means.
- **Gutter markers.** A mark beside a line is a problem on that line: a syntax or
  schema error, a feature the engine can't model, or an error the engine itself
  reported. Hover the mark for the message.
- **Load template…** (in the Policy bar) replaces the editor with a starting
  point such as *Default-deny + explicit grants* or *Tag-based tiers*.
- **`tests` and `sshTests`** blocks are assertions about your own policy
  ("carol can reach `tag:web:443`, but not `tag:db:5432`"). They are how the
  lessons check you — and you can write them in your own policies too.

> [!tip] Break it on purpose.
> Delete a closing bracket or misspell a key and watch the gutter. Learning what
> the errors look like is half of writing policy.

### 2. The Tailnet panel

[Show me the Tailnet panel](show:tailnet)

The **Tailnet** panel is all your devices and anything that you have in your test environment. Simply add nodes, delete nodes, routes to match your actual environment so your policies become accurate.

- **Seed** buttons drop in a ready-made tailnet (*Small startup*, *Multi-team*,
  *Posture mix*, a 30-device *Large* one…).
- **Devices** — name, owning user, `tags`, IP addresses (leave blank to
  auto-assign), operating system, advertised **routes**, and **posture**
  attributes such as the Tailscale version.
- **Users** — email and role (blank is a plain member; or owner, admin,
  it-admin…), which is what `autogroup:admin` and friends match against.
- **Groups** and **Hosts** — the groups and host aliases that exist in the tailnet.
- **Tailscale Services** and **External tailnets** — virtual services and shared
  devices from other tailnets.

Try it: add a device, give it `tag:web`, and watch a new row and column appear in
the matrix.

### 3. The Matrix (Not the movie!)

[Show me: carol-laptop → web-1 on https](show:matrix)

The matrix is the answer, in a grid. Read it as **row → column**: a green cell
means the row's device can open a connection to the column's device.

- The buttons above it choose the **probe** — the protocol and port being
  tested (`https` is tcp/443, `ssh` is tcp/22, `postgres`, `dns`, `icmp`…).
  **probes ⋯** lets you add your own, such as `tcp:8080` or `udp:53`.
- **ssh:ubuntu** (and other `ssh:` buttons) switch to **SSH** results for that
  login user: *accept* connects directly, *check* needs browser re-authentication,
  *reject*/*deny* is blocked. Type a name into **+ ssh user** to test another login.
- **By owner / By tag / Flat** regroup the rows and columns so a 30-device tailnet
  stays readable.
- **Click a cell** to open the *pair detail* below it: the verdict for every
  probe, the SSH result, any application capabilities granted, and the two
  endpoints. Click the cell again to close it.
- **Click a device name** to open the *device inspector*: that device's tags,
  routes, posture, node capabilities, the compiled **packet filter** the
  control plane would send it, and its inbound SSH rules.

[Show me the SSH view](show:ssh)

[Show me web-1’s device inspector](show:inspector)

> [!why] The packet filter is the ground truth.
> The compiled filter is exactly what each device enforces. When a cell isn't
> what you expected, the filter on the destination device shows which rule
> produced — or failed to produce — the access.

Below the grid, collapsible panels appear when your policy uses a feature that
adds its own layer: **Device posture**, **Routes & exit nodes**, **Tailscale Services**
and **Declarative node sharing**. Each says what it changed and why.

### 4. The Graph

[Show me the graph](show:graph)

Click **Graph** on the right for the same result drawn as a network.

- Devices are the nodes. Arrows point from the device that opens the connection
  to the one that receives it.
- **Hover** a node to trace its flows; **click** it to pin the focus. Click the
  background to release.
- **any** shows an edge if the pair can talk on *any* probe, not just the
  selected one.
- **dim routine flows**, **hide isolated** and **highlight group…** are
  declutter controls. **By owner / By tag / Flat** change how devices are grouped.
- The legend underneath is generated from what's on screen, so it always
  matches.

### 5. Learn: the guided lessons

[Show me the checklist](show:learn)

The **Learn** tab is a track of lessons, from "your first tailnet" up to
subnet routers, services and node sharing. Opening a lesson loads its own policy
and tailnet into the editor and Tailnet panel.

- Most lessons are **graded**. A checklist under the lesson lists the
  assertions from its `tests` block; edit the policy until every one is ✓.
  The lesson is complete when they all pass, and the next-lesson button
  turns blue.
- **Explore** lessons have no checklist — they complete as soon as the policy
  evaluates cleanly, and ask you to poke at something and watch what changes.
- **↻ Reset scenario** (under the lesson text) puts the lesson's starting
  policy and tailnet back, so you can't get stuck.

> [!mistake] Opening a lesson replaces what's in the editor.
> If you built something you want to keep, **Share** it first (below) or copy the
> policy out before you open a lesson.

### 6. Using it as a playground

You never need a lesson. Switch to **Tailnet**, pick a **seed**, pick a
**template** for the policy, and start editing. For a matched policy and tailnet
together, use **Load scenario…** in the top bar — worked examples for
admin-role autogroups, subnet routers and exit nodes, Tailscale Services, custom
posture and node sharing.

A good loop for exploring any idea:

1. Write or change a rule.
2. Look at the **Matrix** — did the cells you expected turn green or red?
3. Click a surprising cell to read the pair detail.
4. Open the destination's **device inspector** to see the compiled rule.
5. Add a `tests` entry so the behaviour you want stays locked in.

### 7. Share, theme and About

[Show me the Share notice](show:share)

- **Share** packs your policy, tailnet, probes and current view into a link.
  You choose between a **long link** (everything is inside the URL; nothing is
  stored) and a **short link** (a copy is stored on the Tailground server for a
  year after it was last opened, readable by anyone with the link). Remove
  sensitive names, emails and comments before sharing. Anyone who opens a link is
  asked before it loads.
- The **theme** button cycles light, dark and auto. [Show me](show:theme)
- **about** explains the engine, what Tailground models itself, and what isn't
  simulated. [Show me](show:about)

### Try it now

This lesson's tailnet already has a working policy. Without leaving this page:

1. Open **Matrix**, pick the **https** probe, and click the **carol-laptop → web-1** cell: the pair detail
   lists the verdict for each probe. Then click **web-1** in a column header to
   see its compiled packet filter.
2. In the policy, change the first grant's `"tcp:443"` to `"tcp:8443"`. The
   **https** column goes red and the lesson's check turns ✗ — then put it back.
3. Click the **ssh:ubuntu** button: only alice's and bob's laptops get *check*
   to the web servers (SSH also needs the `tcp:22` grant above it).
4. Switch to **Graph**, hover **web-1**, then use **highlight group…** to ring
   everyone in `group:eng`.
5. Click **Share**, read the notice, and choose a long link.

When you're done, continue to the next lesson and write your first rule.
