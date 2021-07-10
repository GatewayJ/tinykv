// Copyright 2015 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
SoftState	SoftState	软状态，软状态易变且不需要保存在WAL日志中的状态数据，包括：集群leader、节点的当前状态
HardState	HardState	硬状态，与软状态相反，需要写入持久化存储中，包括：节点当前Term、Vote、Commit
ReadStates	[]ReadStates	用于读一致性的数据，后续会详细介绍
Entries	[]pb.Entry	在向其他集群发送消息之前需要先写入持久化存储的日志数据
Snapshot	pb.Snapshot	需要写入持久化存储中的快照数据
CommittedEntries	[]pb.Entry	需要输入到状态机中的数据，这些数据之前已经被保存到持久化存储中了
Messages	[]pb.Message	在entries被写入持久化存储中以后，需要发送出去的数据
————————————————

Package raft sends and receives messages in the Protocol Buffer format
defined in the eraftpb package.

Raft is a protocol with which a cluster of nodes can maintain a replicated state machine.
The state machine is kept in sync through the use of a replicated log.
For more details on Raft, see "In Search of an Understandable Consensus Algorithm"
(https://ramcloud.stanford.edu/raft.pdf) by Diego Ongaro and John Ousterhout.

Usage

The primary object in raft is a Node. You either start a Node from scratch
using raft.StartNode or start a Node from some initial state using raft.RestartNode.

To start a node from scratch:

  storage := raft.NewMemoryStorage()
  c := &Config{
    ID:              0x01,
    ElectionTick:    10,
    HeartbeatTick:   1,
    Storage:         storage,
  }
  n := raft.StartNode(c, []raft.Peer{{ID: 0x02}, {ID: 0x03}})

To restart a node from previous state:

  storage := raft.NewMemoryStorage()

  // recover the in-memory storage from persistent
  // snapshot, state and entries.
  storage.ApplySnapshot(snapshot)
  storage.SetHardState(state)
  storage.Append(entries)

  c := &Config{
    ID:              0x01,
    ElectionTick:    10,
    HeartbeatTick:   1,
    Storage:         storage,
    MaxInflightMsgs: 256,
  }

  // restart raft without peer information.
  // peer information is already included in the storage.
  n := raft.RestartNode(c)

Now that you are holding onto a Node you have a few responsibilities:

First, you must read from the Node.Ready() channel and process the updates
it contains. These steps may be performed in parallel, except as noted in step
2.

首先 你必须从Node.Ready中读取然后处理它包含的更新，这些步骤可以是并行的，除了步骤二中提到的


1. Write HardState, Entries, and Snapshot to persistent storage if they are
not empty. Note that when writing an Entry with Index i, any
previously-persisted entries with Index >= i must be discarded.


当HardState, Entries, 和 Snapshot非空时，将他们持久化到存储中。
注意将一个Entry使用index写入时,之前持久化的index大于1的entry的都必须被忽略


2. Send all Messages to the nodes named in the To field. It is important that
no messages be sent until the latest HardState has been persisted to disk,
and all Entries written by any previous Ready batch (Messages may be sent while
entries from the same batch are being persisted).

重要的是，直到最新的HardState被持久化到磁盘，以及之前任何Ready批处理写入的所有Entries,都不能发送任何消息
（在持久化来自同一批处理的entry时，可能会发送消息）




Note: Marshalling messages is not thread-safe; it is important that you
make sure that no new entries are persisted while marshalling.
The easiest way to achieve this is to serialize the messages directly inside
your main raft loop.

注意 序列化消息不是线程安全的，所以确定在序列化的时候没有entry被持久化是重要的
实现此目的最简单的方法是在主筏循环中直接序列化消息


3. Apply Snapshot (if any) and CommittedEntries to the state machine.
If any committed Entry has Type EntryType_EntryConfChange, call Node.ApplyConfChange()
to apply it to the node. The configuration change may be cancelled at this point
by setting the NodeId field to zero before calling ApplyConfChange
(but ApplyConfChange must be called one way or the other, and the decision to cancel
must be based solely on the state machine and not external information such as
the observed health of the node).


执行快照和提交Entry到状态机
如果提交的entry拥有 EntryType_EntryConfChange字段，则调用Node.ApplyConfChange()来申请节点。
此时可以在调用 ApplyConfChange之前设置NodeId字段为0来取消配置的更改

但是必须以一种或另一种方式调用ApplyConfChange，取消的决定必须完全基于状态机，而不是外部信息，如观察到的节点健康状况




4. Call Node.Advance() to signal readiness for the next batch of updates.
This may be done at any time after step 1, although all updates must be processed
in the order they were returned by Ready.

调用Node.Advance()来准备好进行下一批次的更新
他可以在第一步骤之后的任何时间执行，虽然所有的更新必须按照ready返回的顺序来处理


Second, all persisted log entries must be made available via an
implementation of the Storage interface. The provided MemoryStorage
type can be used for this (if you repopulate its state upon a
restart), or you can supply your own disk-backed implementation.


第二 所有持久化的了日志entry不许通过实现Storage接口来可用。
提供的MemoryStorage可以被用来实现这个，（如果他的状态在重启的时候重新注入），或者你提供自己的存储实现


Third, when you receive a message from another node, pass it to Node.Step:


第三当你收到来自其他节点的meaage时 执行n.Step

	func recvRaftRPC(ctx context.Context, m eraftpb.Message) {
		n.Step(ctx, m)
	}

Finally, you need to call Node.Tick() at regular intervals (probably
via a time.Ticker). Raft has two important timeouts: heartbeat and the
election timeout. However, internally to the raft package time is
represented by an abstract "tick".

最后，你需要定期调用 Node.Tick()，（也可以通过 time.Ticker），Raft有两个重要的超时：
心跳和选举超时。不过内部的时间被抽象的表示为tick



The total state machine handling loop will look something like this:

总的状态机循环看起来像这样


  for {
    select {
    case <-s.Ticker:
      n.Tick()
    case rd := <-s.Node.Ready():
      saveToStorage(rd.State, rd.Entries, rd.Snapshot)
      send(rd.Messages)
      if !raft.IsEmptySnap(rd.Snapshot) {
        processSnapshot(rd.Snapshot)
      }
      for _, entry := range rd.CommittedEntries {
        process(entry)
        if entry.Type == eraftpb.EntryType_EntryConfChange {
          var cc eraftpb.ConfChange
          cc.Unmarshal(entry.Data)
          s.Node.ApplyConfChange(cc)
        }
      }
      s.Node.Advance()
    case <-s.done:
      return
    }
  }

To propose changes to the state machine from your node take your application
data, serialize it into a byte slice and call:

propose从你的节点开始改变状态机，获取应用数据并序列化


	n.Propose(data)

If the proposal is committed, data will appear in committed entries with type
eraftpb.EntryType_EntryNormal. There is no guarantee that a proposed command will be
committed; you may have to re-propose after a timeout.

如果proposal被提交，数据将会携带eraftpb.EntryType_EntryNormal类型出现在已提交的entry内。
并不能保证每一个propose都被提交 ，你可以在超时一段时间后重试




To add or remove a node in a cluster, build ConfChange struct 'cc' and call:

	n.ProposeConfChange(cc)

After config change is committed, some committed entry with type
eraftpb.EntryType_EntryConfChange will be returned. You must apply it to node through:

在配置被提交之后，一些已经提交的entry将带着eraftpb.EntryType_EntryConfChange类型返回，你必须要应用他们到节点


	var cc eraftpb.ConfChange
	cc.Unmarshal(data)
	n.ApplyConfChange(cc)

Note: An ID represents a unique node in a cluster for all time. A
given ID MUST be used only once even if the old node has been removed.
This means that for example IP addresses make poor node IDs since they
may be reused. Node IDs must be non-zero.

一个ID代表一个唯一的节点,一个id只能使用一次即使旧的节点已经被删除也不能使用
这意味ip地址会使节点变得很差，应为他们可能会被重用，所以节点id必须非零




Implementation notes

实施的注意事项


This implementation is up to date with the final Raft thesis
(https://ramcloud.stanford.edu/~ongaro/thesis.pdf), although our
implementation of the membership change protocol differs somewhat from
that described in chapter 4. The key invariant that membership changes
happen one node at a time is preserved, but in our implementation the
membership change takes effect when its entry is applied, not when it
is added to the log (so the entry is committed under the old
membership instead of the new). This is equivalent in terms of safety,
since the old and new configurations are guaranteed to overlap.


这个实现是raft论文最新的，尽管身份变更和第四章描述的略有不同。

虽然我们的实现和论文的第四章略有不同。成员关系每次只发生一个节点的关键不变式被保留，但在我们的实现中，成员关系变化在应用其条目时才生效，而不是在将其添加到日志中时生效
所以一个entry在新的membership替代旧的时候提交。这在安全性方面是等价的，因为新旧配置保证重叠。





To ensure that we do not attempt to commit two membership changes at
once by matching log positions (which would be unsafe since they
should have different quorum requirements), we simply disallow any
proposed membership change while any uncommitted change appears in
the leader's log.


确保我们不会再一次比较日志位置时提交两个membership的更改。（这样是不安全的，因为他们应该有不同的法定票数）

当有未提交的改变在leader的日志中时，我们简单的不允许任何membership更改。



This approach introduces a problem when you try to remove a member
from a two-member cluster: If one of the members dies before the
other one receives the commit of the confchange entry, then the member
cannot be removed any more since the cluster cannot make progress.
For this reason it is highly recommended to use three or more nodes in
every cluster.

当你尝试从两个节点的集群中删除一个节点时这种方法会引入一个问题：如果一个成员在在另一个成员收到配置变更的提交之前死掉的话，由于集群无法取得进展，因此不能再删除成员。
由于这个原因高度推荐一个集群内要有三个或者三个以上的节点

MessageType

Package raft sends and receives message in Protocol Buffer format (defined
in eraftpb package). Each state (follower, candidate, leader) implements its
own 'step' method ('stepFollower', 'stepCandidate', 'stepLeader') when
advancing with the given eraftpb.Message. Each step is determined by its
eraftpb.MessageType. Note that every step is checked by one common method
'Step' that safety-checks the terms of node and incoming message to prevent
stale log entries:

raft包用pb格式发送和接收消息。每一个身份(follower, candidate, leader) 实现他自己的方法('stepFollower', 'stepCandidate', 'stepLeader')，使用eraftpb.Message。
。请注意，每个步骤都由一个通用方法“step”进行检查，该方法安全检查节点和传入消息的术语，以防止日志条目过时


	'MessageType_MsgHup' is used for election. If a node is a follower or candidate, the
	'tick' function in 'raft' struct is set as 'tickElection'. If a follower or
	candidate has not received any heartbeat before the election timeout, it
	passes 'MessageType_MsgHup' to its Step method and becomes (or remains) a candidate to
	start a new election.

	'MessageType_MsgBeat' is an internal type that signals the leader to send a heartbeat of
	the 'MessageType_MsgHeartbeat' type. If a node is a leader, the 'tick' function in
	the 'raft' struct is set as 'tickHeartbeat', and triggers the leader to
	send periodic 'MessageType_MsgHeartbeat' messages to its followers.

	'MessageType_MsgPropose' proposes to append data to its log entries. This is a special
	type to redirect proposals to the leader. Therefore, send method overwrites
	eraftpb.Message's term with its HardState's term to avoid attaching its
	local term to 'MessageType_MsgPropose'. When 'MessageType_MsgPropose' is passed to the leader's 'Step'
	method, the leader first calls the 'appendEntry' method to append entries
	to its log, and then calls 'bcastAppend' method to send those entries to
	its peers. When passed to candidate, 'MessageType_MsgPropose' is dropped. When passed to
	follower, 'MessageType_MsgPropose' is stored in follower's mailbox(msgs) by the send
	method. It is stored with sender's ID and later forwarded to the leader by
	rafthttp package.

	'MessageType_MsgAppend' contains log entries to replicate. A leader calls bcastAppend,
	which calls sendAppend, which sends soon-to-be-replicated logs in 'MessageType_MsgAppend'
	type. When 'MessageType_MsgAppend' is passed to candidate's Step method, candidate reverts
	back to follower, because it indicates that there is a valid leader sending
	'MessageType_MsgAppend' messages. Candidate and follower respond to this message in
	'MessageType_MsgAppendResponse' type.

	'MessageType_MsgAppendResponse' is response to log replication request('MessageType_MsgAppend'). When
	'MessageType_MsgAppend' is passed to candidate or follower's Step method, it responds by
	calling 'handleAppendEntries' method, which sends 'MessageType_MsgAppendResponse' to raft
	mailbox.

	'MessageType_MsgRequestVote' requests votes for election. When a node is a follower or
	candidate and 'MessageType_MsgHup' is passed to its Step method, then the node calls
	'campaign' method to campaign itself to become a leader. Once 'campaign'
	method is called, the node becomes candidate and sends 'MessageType_MsgRequestVote' to peers
	in cluster to request votes. When passed to the leader or candidate's Step
	method and the message's Term is lower than leader's or candidate's,
	'MessageType_MsgRequestVote' will be rejected ('MessageType_MsgRequestVoteResponse' is returned with Reject true).
	If leader or candidate receives 'MessageType_MsgRequestVote' with higher term, it will revert
	back to follower. When 'MessageType_MsgRequestVote' is passed to follower, it votes for the
	sender only when sender's last term is greater than MessageType_MsgRequestVote's term or
	sender's last term is equal to MessageType_MsgRequestVote's term but sender's last committed
	index is greater than or equal to follower's.

	'MessageType_MsgRequestVoteResponse' contains responses from voting request. When 'MessageType_MsgRequestVoteResponse' is
	passed to candidate, the candidate calculates how many votes it has won. If
	it's more than majority (quorum), it becomes leader and calls 'bcastAppend'.
	If candidate receives majority of votes of denials, it reverts back to
	follower.

	'MessageType_MsgSnapshot' requests to install a snapshot message. When a node has just
	become a leader or the leader receives 'MessageType_MsgPropose' message, it calls
	'bcastAppend' method, which then calls 'sendAppend' method to each
	follower. In 'sendAppend', if a leader fails to get term or entries,
	the leader requests snapshot by sending 'MessageType_MsgSnapshot' type message.

	'MessageType_MsgHeartbeat' sends heartbeat from leader. When 'MessageType_MsgHeartbeat' is passed
	to candidate and message's term is higher than candidate's, the candidate
	reverts back to follower and updates its committed index from the one in
	this heartbeat. And it sends the message to its mailbox. When
	'MessageType_MsgHeartbeat' is passed to follower's Step method and message's term is
	higher than follower's, the follower updates its leaderID with the ID
	from the message.

	'MessageType_MsgHeartbeatResponse' is a response to 'MessageType_MsgHeartbeat'. When 'MessageType_MsgHeartbeatResponse'
	is passed to the leader's Step method, the leader knows which follower
	responded.

*/
package raft
